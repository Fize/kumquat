package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fize/kumquat/api/pkg/dto"
	"github.com/fize/kumquat/api/pkg/model"
	appsv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/apps/v1alpha1"
	storagev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
	workspacev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/workspace/v1alpha1"
	"github.com/fize/kumquat/engine/pkg/constants"
	enginelabels "github.com/fize/kumquat/engine/pkg/util/labels"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ErrParentImmutable = errors.New("parent association is immutable")
var ErrWorkspaceNamespaceInUse = errors.New("workspace namespace is immutable while applications exist")
var ErrForbidden = errors.New("resource is outside the actor scope")
var ErrIdempotencyConflict = errors.New("idempotency key was already used for a different request")
var ErrResourceArchived = errors.New("resource is archived")
var ErrConcurrentModification = errors.New("resource changed concurrently; retry with a new idempotency key")
var ErrParentUnavailable = errors.New("parent workspace is not active")

type Principal struct {
	UserID         uint
	RoleID         uint
	RoleName       string
	ModulePublicID string
	Admin          bool
}

type principalContextKey struct{}

func ContextWithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, p)
}

// PrincipalForUser derives scope from durable user/module records. HTTP
// handlers must use this rather than accepting ownership fields from clients.
func (s *ResourceGateway) PrincipalForUser(ctx context.Context, userID uint, _ string) (Principal, error) {
	var user model.User
	if err := s.db.WithContext(ctx).Preload("Role").Preload("Module").First(&user, userID).Error; err != nil {
		return Principal{}, err
	}
	p := Principal{UserID: user.ID, RoleID: user.RoleID, RoleName: user.Role.Name, Admin: user.Role.Name == model.RoleAdmin}
	if p.Admin {
		return p, nil
	}
	if user.ModuleID == nil || user.Module == nil || user.Module.PublicID == "" {
		return Principal{}, ErrForbidden
	}
	p.ModulePublicID = user.Module.PublicID
	return p, nil
}

// PrincipalFromContext returns the server-derived business principal. Callers
// must never synthesize this value from request parameters.
func PrincipalFromContext(ctx context.Context) Principal {
	if p, ok := ctx.Value(principalContextKey{}).(Principal); ok {
		return p
	}
	// Background controller/tests are trusted internal callers. Northbound
	// requests always pass through the authenticated handler context.
	return Principal{Admin: true, RoleName: model.RoleAdmin}
}

func principalFrom(ctx context.Context) Principal { return PrincipalFromContext(ctx) }

func scopeDB(db *gorm.DB, p Principal) *gorm.DB {
	if p.Admin {
		return db
	}
	return db.Where("module_public_id = ?", p.ModulePublicID)
}

func resourceActive(db *gorm.DB) *gorm.DB { return db.Where("archived_at IS NULL") }

func requestFingerprint(actor Principal, route, action, target string, payload any) string {
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s\x00%s", actor.UserID, route, action, target, b)))
	return hex.EncodeToString(sum[:])
}

func desiredDigest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func activeIdentity(kind, name, namespace string) string {
	if kind == model.ResourceApplication {
		return kind + ":" + namespace + ":" + name
	}
	if kind == model.ResourceWorkspace {
		return kind + ":namespace:" + namespace
	}
	return kind + ":" + name
}

type ResourceInput struct {
	Name        string           `json:"name"`
	ProjectID   string           `json:"projectId,omitempty"`
	WorkspaceID string           `json:"workspaceId,omitempty"`
	Desired     dto.DesiredState `json:"desired" binding:"required"`
}

type ResourceGateway struct {
	db            *gorm.DB
	k8s           client.Client
	mu            sync.RWMutex
	reconcileMu   sync.Mutex
	workerStarted bool
	lastLoopError error
}

func NewResourceGateway(db *gorm.DB, k8s client.Client) *ResourceGateway {
	return &ResourceGateway{db: db, k8s: k8s}
}

func newPublicID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func (s *ResourceGateway) Create(ctx context.Context, kind string, in ResourceInput, idempotencyKey string) (*model.Operation, error) {
	actor := principalFrom(ctx)
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if err := validateDesired(kind, in.Desired, false); err != nil {
		return nil, err
	}
	desired, err := json.Marshal(in.Desired)
	if err != nil {
		return nil, fmt.Errorf("encode desired state: %w", err)
	}
	if idempotencyKey == "" {
		idempotencyKey = newPublicID("idem")
	}
	route := "/api/v1/" + kind + "s"
	fingerprint := requestFingerprint(actor, route, "create", "", in)
	var result model.Operation
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if found, e := findIdempotent(tx, actor.UserID, idempotencyKey, fingerprint, &result); found || e != nil {
			return e
		}
		record := model.ResourceRecord{ID: newPublicID(kind), Kind: kind, Name: in.Name, EngineName: in.Name, DesiredJSON: string(desired), DesiredHash: desiredDigest(string(desired)), DesiredRevision: 1, SyncState: "pending", Source: "api"}
		switch kind {
		case model.ResourceWorkspace:
			if in.ProjectID == "" {
				return errors.New("projectId is required")
			}
			var project model.Project
			query := tx.Preload("Module").Where("projects.public_id = ?", in.ProjectID)
			if !actor.Admin {
				query = query.Joins("JOIN modules ON modules.id = projects.module_id").Where("modules.public_id = ?", actor.ModulePublicID)
			}
			if err := query.First(&project).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("project not found")
			} else if err != nil {
				return err
			}
			record.ProjectID = &project.ID
			record.ModuleID = &project.ModuleID
			record.ProjectPublicID = project.PublicID
			record.ModulePublicID = project.Module.PublicID
			record.ParentID = project.PublicID
			namespace := in.Name
			if in.Desired.Workspace != nil && in.Desired.Workspace.Namespace != "" {
				namespace = in.Desired.Workspace.Namespace
			}
			record.EngineNamespace = namespace
		case model.ResourceApplication:
			if in.WorkspaceID == "" {
				return errors.New("workspaceId is required")
			}
			var parent model.ResourceRecord
			query := resourceActive(scopeDB(tx, actor)).Where("id = ? AND kind = ? AND sync_state <> ? AND state <> ?", in.WorkspaceID, model.ResourceWorkspace, "deleting", "archived")
			if err := query.First(&parent).Error; err != nil {
				return errors.New("workspace not found")
			}
			namespace, err := effectiveWorkspaceNamespace(parent)
			if err != nil {
				return err
			}
			record.ParentID = parent.ID
			record.ProjectID = parent.ProjectID
			record.ModuleID = parent.ModuleID
			record.ProjectPublicID = parent.ProjectPublicID
			record.ModulePublicID = parent.ModulePublicID
			record.EngineNamespace = namespace
		default:
			return fmt.Errorf("unsupported resource kind %q", kind)
		}
		key := activeIdentity(kind, record.EngineName, record.EngineNamespace)
		record.ActiveKey = &key
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		result = model.Operation{ID: newPublicID("op"), IdempotencyKey: idempotencyKey, ActorID: actor.UserID, ModulePublicID: record.ModulePublicID, Route: route, Fingerprint: fingerprint, ResourceID: record.ID, Action: "apply", State: "pending"}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		return tx.Create(&model.OutboxEvent{ID: newPublicID("evt"), OperationID: result.ID, ResourceID: record.ID, Action: "apply", DesiredRevision: record.DesiredRevision, AvailableAt: time.Now()}).Error
	})
	if err != nil && isUniqueConflict(err) {
		if found, replayErr := findIdempotent(s.db.WithContext(ctx), actor.UserID, idempotencyKey, fingerprint, &result); found || replayErr != nil {
			return &result, replayErr
		}
	}
	return &result, err
}

func findIdempotent(db *gorm.DB, actorID uint, key, fingerprint string, op *model.Operation) (bool, error) {
	err := db.Where("actor_id = ? AND idempotency_key = ?", actorID, key).First(op).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if op.Fingerprint != fingerprint {
		return true, ErrIdempotencyConflict
	}
	return true, nil
}

func isUniqueConflict(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unique constraint") || strings.Contains(s, "duplicate key")
}

func (s *ResourceGateway) Update(ctx context.Context, id, kind string, in ResourceInput, idempotencyKey string) (*model.Operation, error) {
	actor := principalFrom(ctx)
	var record model.ResourceRecord
	if err := resourceActive(scopeDB(s.db.WithContext(ctx), actor)).Where("id = ? AND kind = ? AND sync_state <> ? AND state <> ?", id, kind, "deleting", "archived").First(&record).Error; err != nil {
		return nil, err
	}
	expectedParent := in.WorkspaceID
	if kind == model.ResourceWorkspace {
		expectedParent = in.ProjectID
	}
	if expectedParent != "" && expectedParent != record.ParentID {
		return nil, ErrParentImmutable
	}
	if err := validateDesired(kind, in.Desired, true); err != nil {
		return nil, err
	}
	if in.Name != "" && in.Name != record.Name {
		return nil, errors.New("resource name is immutable")
	}
	if kind == model.ResourceWorkspace && hasDesired(in.Desired) {
		currentNamespace, err := effectiveWorkspaceNamespace(record)
		if err != nil {
			return nil, err
		}
		candidate := record
		encoded, err := json.Marshal(in.Desired)
		if err != nil {
			return nil, err
		}
		candidate.DesiredJSON = string(encoded)
		newNamespace, err := effectiveWorkspaceNamespace(candidate)
		if err != nil {
			return nil, err
		}
		if currentNamespace != newNamespace {
			var children int64
			if err := s.db.WithContext(ctx).Model(&model.ResourceRecord{}).Where("parent_id = ? AND kind = ? AND archived_at IS NULL", record.ID, model.ResourceApplication).Count(&children).Error; err != nil {
				return nil, err
			}
			if children > 0 {
				return nil, ErrWorkspaceNamespaceInUse
			}
		}
	}
	if idempotencyKey == "" {
		idempotencyKey = newPublicID("idem")
	}
	route := "/api/v1/" + kind + "s/" + id
	fingerprint := requestFingerprint(actor, route, "update", id, in)
	var op model.Operation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if found, e := findIdempotent(tx, actor.UserID, idempotencyKey, fingerprint, &op); found || e != nil {
			return e
		}
		if hasDesired(in.Desired) {
			desired, marshalErr := json.Marshal(in.Desired)
			if marshalErr != nil {
				return marshalErr
			}
			record.DesiredJSON = string(desired)
			record.DesiredHash = desiredDigest(string(desired))
			record.DesiredRevision++
		}
		updates := map[string]any{"desired_json": record.DesiredJSON, "desired_hash": record.DesiredHash, "desired_revision": record.DesiredRevision, "sync_state": "pending"}
		if kind == model.ResourceWorkspace && hasDesired(in.Desired) {
			candidate := record
			candidate.DesiredJSON = record.DesiredJSON
			namespace, e := effectiveWorkspaceNamespace(candidate)
			if e != nil {
				return e
			}
			key := activeIdentity(kind, record.EngineName, namespace)
			updates["engine_namespace"] = namespace
			updates["active_key"] = key
		}
		previousRevision := record.DesiredRevision
		if hasDesired(in.Desired) {
			previousRevision--
		}
		result := tx.Model(&model.ResourceRecord{}).Where("id = ? AND archived_at IS NULL AND desired_revision = ? AND sync_state <> ? AND state <> ?", record.ID, previousRevision, "deleting", "archived").Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConcurrentModification
		}
		op = model.Operation{ID: newPublicID("op"), IdempotencyKey: idempotencyKey, ActorID: actor.UserID, ModulePublicID: record.ModulePublicID, Route: route, Fingerprint: fingerprint, ResourceID: id, Action: "apply", State: "pending"}
		if err := tx.Create(&op).Error; err != nil {
			return err
		}
		return tx.Create(&model.OutboxEvent{ID: newPublicID("evt"), OperationID: op.ID, ResourceID: id, Action: "apply", DesiredRevision: record.DesiredRevision, AvailableAt: time.Now()}).Error
	})
	if err != nil && isUniqueConflict(err) {
		if found, replayErr := findIdempotent(s.db.WithContext(ctx), actor.UserID, idempotencyKey, fingerprint, &op); found || replayErr != nil {
			return &op, replayErr
		}
	}
	return &op, err
}

func (s *ResourceGateway) Delete(ctx context.Context, id, kind, idempotencyKey string) (*model.Operation, error) {
	actor := principalFrom(ctx)
	if idempotencyKey == "" {
		idempotencyKey = newPublicID("idem")
	}
	route := "/api/v1/" + kind + "s/" + id
	fingerprint := requestFingerprint(actor, route, "delete", id, nil)
	var op model.Operation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if found, e := findIdempotent(tx, actor.UserID, idempotencyKey, fingerprint, &op); found || e != nil {
			return e
		}
		var record model.ResourceRecord
		if err := resourceActive(scopeDB(tx, actor)).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND kind = ?", id, kind).First(&record).Error; err != nil {
			return err
		}
		var children int64
		if err := resourceActive(tx.Model(&model.ResourceRecord{})).Where("parent_id = ?", id).Count(&children).Error; err != nil {
			return err
		}
		if children > 0 {
			return errors.New("cannot delete resource with children")
		}
		record.SyncState = "deleting"
		result := tx.Model(&model.ResourceRecord{}).Where("id = ? AND archived_at IS NULL AND desired_revision = ? AND sync_state <> ?", id, record.DesiredRevision, "deleting").Update("sync_state", "deleting")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConcurrentModification
		}
		op = model.Operation{ID: newPublicID("op"), IdempotencyKey: idempotencyKey, ActorID: actor.UserID, ModulePublicID: record.ModulePublicID, Route: route, Fingerprint: fingerprint, ResourceID: id, Action: "delete", State: "pending"}
		if err := tx.Create(&op).Error; err != nil {
			return err
		}
		return tx.Create(&model.OutboxEvent{ID: newPublicID("evt"), OperationID: op.ID, ResourceID: id, Action: "delete", DesiredRevision: record.DesiredRevision, AvailableAt: time.Now()}).Error
	})
	if err != nil && isUniqueConflict(err) {
		if found, replayErr := findIdempotent(s.db.WithContext(ctx), actor.UserID, idempotencyKey, fingerprint, &op); found || replayErr != nil {
			return &op, replayErr
		}
	}
	return &op, err
}

func recordView(r model.ResourceRecord) (dto.ResourceView, error) {
	var desired dto.DesiredState
	if r.DesiredJSON != "" {
		if err := json.Unmarshal([]byte(r.DesiredJSON), &desired); err != nil {
			return dto.ResourceView{}, fmt.Errorf("decode desired projection: %w", err)
		}
	}
	if desired.Application != nil {
		for i := range desired.Application.Template.Containers {
			for j := range desired.Application.Template.Containers[i].Env {
				desired.Application.Template.Containers[i].Env[j].Value = ""
			}
		}
		for i := range desired.Application.Overrides {
			for j := range desired.Application.Overrides[i].Env {
				desired.Application.Overrides[i].Env[j].Value = ""
			}
		}
	}
	var observed dto.ObservedState
	if r.ObservedJSON != "" {
		if err := json.Unmarshal([]byte(r.ObservedJSON), &observed); err != nil {
			return dto.ResourceView{}, fmt.Errorf("decode observed projection: %w", err)
		}
	}
	return dto.ResourceView{ID: r.ID, Kind: r.Kind, Name: r.Name, ParentID: r.ParentID, ProjectID: r.ProjectPublicID, ModuleID: r.ModulePublicID, Desired: desired, Observed: observed, ObservedAt: r.ObservedAt, SyncState: r.SyncState, Source: r.Source, State: r.State}, nil
}
func (s *ResourceGateway) Get(ctx context.Context, id, kind string) (*dto.ResourceView, error) {
	actor := principalFrom(ctx)
	var r model.ResourceRecord
	if err := resourceActive(scopeDB(s.db.WithContext(ctx), actor)).Where("id = ? AND kind = ?", id, kind).First(&r).Error; err != nil {
		return nil, err
	}
	v, err := recordView(r)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
func (s *ResourceGateway) List(ctx context.Context, kind string) ([]dto.ResourceView, error) {
	actor := principalFrom(ctx)
	var rs []model.ResourceRecord
	query := resourceActive(scopeDB(s.db.WithContext(ctx), actor)).Where("kind = ?", kind)
	if kind == model.ResourceCluster && !actor.Admin {
		query = query.Where("1 = 0")
	}
	if err := query.Order("created_at desc").Find(&rs).Error; err != nil {
		return nil, err
	}
	out := make([]dto.ResourceView, len(rs))
	for i := range rs {
		view, err := recordView(rs[i])
		if err != nil {
			return nil, err
		}
		out[i] = view
	}
	return out, nil
}
func (s *ResourceGateway) Operation(ctx context.Context, id string) (*model.Operation, error) {
	actor := principalFrom(ctx)
	var op model.Operation
	query := s.db.WithContext(ctx).Where("id = ?", id)
	if !actor.Admin {
		query = query.Where("actor_id = ? AND module_public_id = ?", actor.UserID, actor.ModulePublicID)
	}
	if err := query.First(&op).Error; err != nil {
		return nil, err
	}
	return &op, nil
}

func (s *ResourceGateway) AdoptCluster(ctx context.Context, id, key string) (*model.Operation, error) {
	actor := principalFrom(ctx)
	if !actor.Admin {
		return nil, ErrForbidden
	}
	if key == "" {
		key = newPublicID("idem")
	}
	route := "/api/v1/clusters/" + id + "/adopt"
	fingerprint := requestFingerprint(actor, route, "adopt", id, nil)
	var op model.Operation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if found, e := findIdempotent(tx, actor.UserID, key, fingerprint, &op); found || e != nil {
			return e
		}
		var r model.ResourceRecord
		if err := tx.Where("id = ? AND kind = ?", id, model.ResourceCluster).First(&r).Error; err != nil {
			return err
		}
		if r.State != "discovered" {
			return errors.New("cluster is not awaiting adoption")
		}
		result := tx.Model(&model.ResourceRecord{}).Where("id = ? AND state = ? AND archived_at IS NULL", r.ID, "discovered").Updates(map[string]any{"state": "adopting", "sync_state": "pending"})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConcurrentModification
		}
		op = model.Operation{ID: newPublicID("op"), IdempotencyKey: key, ActorID: actor.UserID, Route: route, Fingerprint: fingerprint, ResourceID: id, Action: "adopt", State: "pending"}
		if err := tx.Create(&op).Error; err != nil {
			return err
		}
		return tx.Create(&model.OutboxEvent{ID: newPublicID("evt"), OperationID: op.ID, ResourceID: id, Action: "adopt", DesiredRevision: r.DesiredRevision, AvailableAt: time.Now()}).Error
	})
	if err != nil && isUniqueConflict(err) {
		if found, replayErr := findIdempotent(s.db.WithContext(ctx), actor.UserID, key, fingerprint, &op); found || replayErr != nil {
			return &op, replayErr
		}
	}
	return &op, err
}

// AdoptExisting performs the one-time assignment of a discovered Engine
// object. Once assigned, ParentID remains immutable; this is not a transfer.
func (s *ResourceGateway) AdoptExisting(ctx context.Context, id, kind string, in ResourceInput, key string) (*model.Operation, error) {
	actor := principalFrom(ctx)
	if !actor.Admin {
		return nil, ErrForbidden
	}
	if kind != model.ResourceWorkspace && kind != model.ResourceApplication {
		return nil, errors.New("only discovered workspaces and applications can be adopted")
	}
	if key == "" {
		key = newPublicID("idem")
	}
	route := "/api/v1/" + kind + "s/" + id + "/adopt-existing"
	fingerprint := requestFingerprint(actor, route, "adopt", id, in)
	var op model.Operation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if found, e := findIdempotent(tx, actor.UserID, key, fingerprint, &op); found || e != nil {
			return e
		}
		var r model.ResourceRecord
		if err := resourceActive(tx).Where("id = ? AND kind = ? AND state = ? AND source = ?", id, kind, "unassigned", "engine").First(&r).Error; err != nil {
			return err
		}
		if r.CapabilityError != "" {
			return fmt.Errorf("unsupported Engine application cannot be adopted without loss: %s", r.CapabilityError)
		}
		updates := map[string]any{"sync_state": "pending", "state": "adopting", "source": "api"}
		if kind == model.ResourceWorkspace {
			if in.ProjectID == "" {
				return errors.New("projectId is required")
			}
			var p model.Project
			if err := tx.Preload("Module").Where("public_id = ?", in.ProjectID).First(&p).Error; err != nil {
				return errors.New("project not found")
			}
			updates["parent_id"], updates["project_id"], updates["module_id"], updates["project_public_id"], updates["module_public_id"] = p.PublicID, p.ID, p.ModuleID, p.PublicID, p.Module.PublicID
			r.ModulePublicID = p.Module.PublicID
		} else {
			if in.WorkspaceID == "" {
				return errors.New("workspaceId is required")
			}
			var parent model.ResourceRecord
			parentQuery := resourceActive(tx).Clauses(clause.Locking{Strength: "UPDATE"}).Where(
				"id = ? AND kind = ? AND sync_state <> ? AND state NOT IN ?",
				in.WorkspaceID, model.ResourceWorkspace, "deleting", []string{"unassigned", "archived"},
			)
			if err := parentQuery.First(&parent).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrParentUnavailable
			} else if err != nil {
				return err
			}
			namespace, e := effectiveWorkspaceNamespace(parent)
			if e != nil {
				return e
			}
			if namespace != r.EngineNamespace {
				return errors.New("application namespace does not match workspace namespace")
			}
			updates["parent_id"], updates["project_id"], updates["module_id"], updates["project_public_id"], updates["module_public_id"] = parent.ID, parent.ProjectID, parent.ModuleID, parent.ProjectPublicID, parent.ModulePublicID
			r.ModulePublicID = parent.ModulePublicID
		}
		adoptionQuery := tx.Model(&model.ResourceRecord{}).Where("id = ? AND state = ? AND parent_id = ''", r.ID, "unassigned")
		// Application adoption already selected the active parent FOR UPDATE in
		// this transaction. That row lock keeps the lifecycle predicate true
		// until commit and avoids MySQL error 1093 (updating a table while a
		// subquery selects from the same table). The child state/empty-parent
		// predicate below still provides the compare-and-set for one-time adoption.
		result := adoptionQuery.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			if kind == model.ResourceApplication {
				var activeParents int64
				if err := tx.Model(&model.ResourceRecord{}).Where(
					"id = ? AND kind = ? AND archived_at IS NULL AND sync_state <> ? AND state NOT IN ?",
					in.WorkspaceID, model.ResourceWorkspace, "deleting", []string{"unassigned", "archived"},
				).Count(&activeParents).Error; err != nil {
					return err
				}
				if activeParents != 1 {
					return ErrParentUnavailable
				}
			}
			return ErrConcurrentModification
		}
		op = model.Operation{ID: newPublicID("op"), IdempotencyKey: key, ActorID: actor.UserID, ModulePublicID: r.ModulePublicID, Route: route, Fingerprint: fingerprint, ResourceID: r.ID, Action: "adopt-existing", State: "pending"}
		if err := tx.Create(&op).Error; err != nil {
			return err
		}
		return tx.Create(&model.OutboxEvent{ID: newPublicID("evt"), OperationID: op.ID, ResourceID: r.ID, Action: "adopt-existing", DesiredRevision: r.DesiredRevision, AvailableAt: time.Now()}).Error
	})
	if err != nil && isUniqueConflict(err) {
		if found, replayErr := findIdempotent(s.db.WithContext(ctx), actor.UserID, key, fingerprint, &op); found || replayErr != nil {
			return &op, replayErr
		}
	}
	return &op, err
}

func (s *ResourceGateway) Run(ctx context.Context) {
	if s.k8s == nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		var loopErr error
		for _, work := range []func(context.Context) error{s.DiscoverClusters, s.DiscoverEngineResources, s.EnsureDesired, s.SyncObserved, s.ReconcileOne} {
			if err := work(ctx); err != nil {
				loopErr = errors.Join(loopErr, err)
			}
		}
		s.mu.Lock()
		s.workerStarted = true
		s.lastLoopError = loopErr
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// EnsureDesired continuously compares the API-owned desired revision with its
// Engine projection. It creates at most one repair event per resource and
// backs off terminal failures to avoid a hot loop.
func (s *ResourceGateway) EnsureDesired(ctx context.Context) error {
	if s.k8s == nil {
		return nil
	}
	var records []model.ResourceRecord
	if err := resourceActive(s.db.WithContext(ctx)).Where("kind IN ? AND source = ? AND sync_state <> ?", []string{model.ResourceWorkspace, model.ResourceApplication}, "api", "deleting").Find(&records).Error; err != nil {
		return err
	}
	for i := range records {
		r := records[i]
		if r.SyncState == "failed" && time.Since(r.UpdatedAt) < 30*time.Second {
			continue
		}
		drifted, err := s.projectionDrifted(ctx, &r)
		if err != nil {
			return err
		}
		if !drifted && r.AppliedRevision >= r.DesiredRevision {
			continue
		}
		if err := s.enqueueRepair(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (s *ResourceGateway) enqueueRepair(ctx context.Context, r model.ResourceRecord) error {
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.OutboxEvent{}).Where("resource_id = ? AND processed_at IS NULL", r.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		op := model.Operation{ID: newPublicID("op"), IdempotencyKey: newPublicID("repair"), Route: "internal/reconcile", Fingerprint: r.DesiredHash, ResourceID: r.ID, Action: "apply", State: "pending", ModulePublicID: r.ModulePublicID}
		if err := tx.Create(&op).Error; err != nil {
			return err
		}
		return tx.Create(&model.OutboxEvent{ID: newPublicID("evt"), OperationID: op.ID, ResourceID: r.ID, Action: "apply", DesiredRevision: r.DesiredRevision, AvailableAt: time.Now()}).Error
	}); err != nil {
		return err
	}
	return nil
}

func (s *ResourceGateway) projectionDrifted(ctx context.Context, r *model.ResourceRecord) (bool, error) {
	var desired dto.DesiredState
	if err := json.Unmarshal([]byte(r.DesiredJSON), &desired); err != nil {
		return false, err
	}
	switch r.Kind {
	case model.ResourceWorkspace:
		var current workspacev1alpha1.Workspace
		if err := s.k8s.Get(ctx, client.ObjectKey{Name: r.EngineName}, &current); apierrors.IsNotFound(err) {
			return true, nil
		} else if err != nil {
			return false, err
		}
		if desired.Workspace == nil {
			return false, errors.New("workspace desired state missing")
		}
		return !reflect.DeepEqual(current.Spec, workspaceSpec(*desired.Workspace)) || !businessLabelsMatch(current.Labels, s.businessLabels(r)), nil
	case model.ResourceApplication:
		var current appsv1alpha1.Application
		if err := s.k8s.Get(ctx, client.ObjectKey{Name: r.EngineName, Namespace: r.EngineNamespace}, &current); apierrors.IsNotFound(err) {
			return true, nil
		} else if err != nil {
			return false, err
		}
		if desired.Application == nil {
			return false, errors.New("application desired state missing")
		}
		expected, err := applicationSpec(*desired.Application)
		if err != nil {
			return false, err
		}
		return !reflect.DeepEqual(current.Spec, expected) || !businessLabelsMatch(current.Labels, s.businessLabels(r)), nil
	}
	return false, nil
}

func businessLabelsMatch(current, expected map[string]string) bool {
	for key, value := range expected {
		if current[key] != value {
			return false
		}
	}
	return true
}

// DiscoverEngineResources imports pre-existing Engine objects as unassigned
// records. It deliberately does not invent Project/Module ownership; explicit
// association remains a separate administrative migration, not a transfer.
func (s *ResourceGateway) DiscoverEngineResources(ctx context.Context) error {
	if s.k8s == nil {
		return nil
	}
	var workspaces workspacev1alpha1.WorkspaceList
	if err := s.k8s.List(ctx, &workspaces); err != nil {
		return err
	}
	for i := range workspaces.Items {
		w := &workspaces.Items[i]
		var count int64
		if err := s.db.WithContext(ctx).Model(&model.ResourceRecord{}).Where("kind = ? AND engine_name = ?", model.ResourceWorkspace, w.Name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		namespace := w.Spec.Name
		if namespace == "" {
			namespace = w.Name
		}
		workspaceInput := workspaceDesired(w.Spec)
		if workspaceInput.Namespace == "" {
			workspaceInput.Namespace = namespace
		}
		desired := dto.DesiredState{Workspace: &workspaceInput}
		raw, _ := json.Marshal(desired)
		key := activeIdentity(model.ResourceWorkspace, w.Name, namespace)
		now := time.Now()
		record := model.ResourceRecord{ID: newPublicID("workspace"), Kind: model.ResourceWorkspace, Name: w.Name, EngineName: w.Name, EngineNamespace: namespace, ActiveKey: &key, DesiredJSON: string(raw), DesiredHash: desiredDigest(string(raw)), DesiredRevision: 1, AppliedRevision: 1, SyncState: "synced", Source: "engine", State: "unassigned", ObservedAt: &now}
		if err := s.db.WithContext(ctx).Create(&record).Error; err != nil && !isUniqueConflict(err) {
			return err
		}
	}
	var applications appsv1alpha1.ApplicationList
	if err := s.k8s.List(ctx, &applications); err != nil {
		return err
	}
	for i := range applications.Items {
		a := &applications.Items[i]
		var count int64
		if err := s.db.WithContext(ctx).Model(&model.ResourceRecord{}).Where("kind = ? AND engine_name = ? AND engine_namespace = ?", model.ResourceApplication, a.Name, a.Namespace).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		desired, adaptationErr := applicationDesired(a.Spec)
		raw := []byte("{}")
		capabilityError, syncState := "", "synced"
		if adaptationErr != nil {
			capabilityError, syncState = adaptationErr.Error(), "unsupported"
		} else {
			raw, _ = json.Marshal(dto.DesiredState{Application: &desired})
		}
		key := activeIdentity(model.ResourceApplication, a.Name, a.Namespace)
		now := time.Now()
		record := model.ResourceRecord{ID: newPublicID("application"), Kind: model.ResourceApplication, Name: a.Name, EngineName: a.Name, EngineNamespace: a.Namespace, ActiveKey: &key, DesiredJSON: string(raw), DesiredHash: desiredDigest(string(raw)), DesiredRevision: 1, AppliedRevision: 1, SyncState: syncState, Source: "engine", State: "unassigned", CapabilityError: capabilityError, ObservedAt: &now}
		if err := s.db.WithContext(ctx).Create(&record).Error; err != nil && !isUniqueConflict(err) {
			return err
		}
	}
	return nil
}

func (s *ResourceGateway) Ready(ctx context.Context) error {
	if s.k8s == nil {
		return errors.New("engine client unavailable")
	}
	if sqlDB, err := s.db.DB(); err != nil {
		return err
	} else if err := sqlDB.PingContext(ctx); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.workerStarted {
		return errors.New("resource worker not started")
	}
	if s.lastLoopError != nil {
		return fmt.Errorf("resource worker unhealthy: %w", s.lastLoopError)
	}
	return nil
}

// SyncObserved refreshes the read projection from Engine status. Desired state
// is never reconstructed from this data.
func (s *ResourceGateway) SyncObserved(ctx context.Context) error {
	if s.k8s == nil {
		return nil
	}
	var records []model.ResourceRecord
	if err := resourceActive(s.db.WithContext(ctx)).Where("kind IN ?", []string{model.ResourceWorkspace, model.ResourceApplication}).Find(&records).Error; err != nil {
		return err
	}
	for i := range records {
		r := &records[i]
		var observed dto.ObservedState
		switch r.Kind {
		case model.ResourceWorkspace:
			var obj workspacev1alpha1.Workspace
			if getErr := s.k8s.Get(ctx, client.ObjectKey{Name: r.EngineName}, &obj); getErr != nil {
				if apierrors.IsNotFound(getErr) {
					if err := s.markMissingAndRepair(ctx, *r); err != nil {
						return err
					}
					continue
				}
				return getErr
			}
			observed.Workspace = pointer(workspaceObserved(obj.Status))
		case model.ResourceApplication:
			var obj appsv1alpha1.Application
			if getErr := s.k8s.Get(ctx, client.ObjectKey{Name: r.EngineName, Namespace: r.EngineNamespace}, &obj); getErr != nil {
				if apierrors.IsNotFound(getErr) {
					if err := s.markMissingAndRepair(ctx, *r); err != nil {
						return err
					}
					continue
				}
				return getErr
			}
			observed.Application = pointer(applicationObserved(obj.Status))
		}
		encoded, err := json.Marshal(observed)
		if err != nil {
			return err
		}
		now := time.Now()
		if err := s.db.WithContext(ctx).Model(&model.ResourceRecord{}).Where("id = ? AND desired_revision = ? AND archived_at IS NULL", r.ID, r.DesiredRevision).Updates(map[string]any{"observed_json": string(encoded), "observed_at": &now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *ResourceGateway) markMissingAndRepair(ctx context.Context, r model.ResourceRecord) error {
	if r.SyncState == "deleting" {
		return nil
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ResourceRecord{}).Where("id = ? AND desired_revision = ? AND archived_at IS NULL", r.ID, r.DesiredRevision).Updates(map[string]any{"sync_state": "stale", "observed_json": "", "observed_at": &now}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if r.Source != "api" {
		return nil
	}
	// EnsureDesired is also called independently by the worker; enqueue here so
	// a direct SyncObserved call has the same repair semantics.
	return s.enqueueRepair(ctx, r)
}

func (s *ResourceGateway) DiscoverClusters(ctx context.Context) error {
	if s.k8s == nil {
		return nil
	}
	var list storagev1alpha1.ManagedClusterList
	if err := s.k8s.List(ctx, &list); err != nil {
		return err
	}
	for i := range list.Items {
		c := &list.Items[i]
		if c.Spec.ConnectionMode != storagev1alpha1.ClusterConnectionModeEdge || c.Labels[constants.LabelRegistrationSource] != constants.RegistrationSourceAgent {
			continue
		}
		observed, err := json.Marshal(dto.ObservedState{Cluster: pointer(clusterObserved(*c))})
		if err != nil {
			return err
		}
		now := time.Now()
		var r model.ResourceRecord
		err = resourceActive(s.db.WithContext(ctx)).Where("kind = ? AND engine_name = ?", model.ResourceCluster, c.Name).First(&r).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			key := activeIdentity(model.ResourceCluster, c.Name, "")
			r = model.ResourceRecord{ID: newPublicID("cluster"), Kind: model.ResourceCluster, Name: c.Name, EngineName: c.Name, ActiveKey: &key, ObservedJSON: string(observed), ObservedAt: &now, SyncState: "synced", Source: "agent", State: "discovered"}
			if err := s.db.WithContext(ctx).Create(&r).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if err := s.db.WithContext(ctx).Model(&model.ResourceRecord{}).Where("id = ?", r.ID).Updates(map[string]any{"observed_json": string(observed), "observed_at": &now}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ResourceGateway) ReconcileOne(ctx context.Context) error {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	token := newPublicID("claim")
	now := time.Now()
	var event model.OutboxEvent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var candidate model.OutboxEvent
		if err := tx.Where("processed_at IS NULL AND available_at <= ? AND (claim_until IS NULL OR claim_until < ?)", now, now).Order("created_at").First(&candidate).Error; err != nil {
			return err
		}
		until := now.Add(30 * time.Second)
		result := tx.Model(&model.OutboxEvent{}).Where("id = ? AND processed_at IS NULL AND (claim_until IS NULL OR claim_until < ?)", candidate.ID, now).Updates(map[string]any{"claim_token": token, "claim_until": &until})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.First(&event, "id = ?", candidate.ID).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var r model.ResourceRecord
	resourceQuery := s.db.WithContext(ctx).Where("id = ?", event.ResourceID)
	switch event.Action {
	case "apply", "adopt-existing":
		resourceQuery = resourceQuery.Where("archived_at IS NULL AND sync_state <> ? AND state <> ? AND source = ? AND desired_revision = ?", "deleting", "archived", "api", event.DesiredRevision)
	case "delete":
		resourceQuery = resourceQuery.Where("archived_at IS NULL AND sync_state = ? AND desired_revision = ?", "deleting", event.DesiredRevision)
	default:
		resourceQuery = resourceQuery.Where("archived_at IS NULL")
	}
	if err := resourceQuery.First(&r).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		var exists int64
		if countErr := s.db.WithContext(ctx).Model(&model.ResourceRecord{}).Where("id = ?", event.ResourceID).Count(&exists).Error; countErr != nil {
			return s.failEvent(&event, countErr)
		}
		if exists == 0 {
			return s.failEvent(&event, gorm.ErrRecordNotFound)
		}
		return s.discardEvent(&event, ErrResourceArchived)
	} else if err != nil {
		return s.failEvent(&event, err)
	}
	if s.k8s == nil {
		return s.failEvent(&event, errors.New("engine client unavailable"))
	}
	err = s.project(ctx, &r, event.Action)
	now = time.Now()
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var claimed model.OutboxEvent
		if e := tx.Where("id = ? AND claim_token = ? AND processed_at IS NULL", event.ID, token).First(&claimed).Error; e != nil {
			return nil
		}
		var op model.Operation
		if e := tx.First(&op, "id = ?", event.OperationID).Error; e != nil {
			return e
		}
		if err != nil {
			claimed.Attempts++
			claimed.LastError = err.Error()
			op.Error = err.Error()
			if claimed.Attempts >= maxOutboxAttempts {
				claimed.ProcessedAt = &now
				op.State = "failed"
				r.SyncState = "failed"
			} else {
				claimed.AvailableAt = time.Now().Add(time.Duration(claimed.Attempts) * time.Second)
				op.State = "retrying"
				r.SyncState = "retrying"
			}
		} else {
			claimed.ProcessedAt = &now
			op.State = "succeeded"
			r.SyncState = "synced"
			if event.Action == "adopt" || event.Action == "adopt-existing" {
				r.State = "adopted"
			} else if event.Action == "delete" {
				r.State = "archived"
				r.ArchivedAt = &now
				r.ActiveKey = nil
			}
		}
		claimed.ClaimToken = ""
		claimed.ClaimUntil = nil
		if e := tx.Save(&claimed).Error; e != nil {
			return e
		}
		if e := tx.Model(&model.Operation{}).Where("id = ? AND state NOT IN ?", op.ID, []string{"succeeded", "failed"}).Updates(map[string]any{"state": op.State, "error": op.Error}).Error; e != nil {
			return e
		}
		updates := map[string]any{"sync_state": r.SyncState, "state": r.State, "archived_at": r.ArchivedAt, "active_key": r.ActiveKey}
		if err == nil && (event.Action == "apply" || event.Action == "adopt-existing") {
			updates["applied_revision"] = r.DesiredRevision
		}
		query := tx.Model(&model.ResourceRecord{}).Where("id = ? AND desired_revision = ?", r.ID, r.DesiredRevision)
		if event.Action == "apply" || event.Action == "adopt-existing" {
			query = query.Where("archived_at IS NULL AND sync_state <> ? AND state <> ? AND source = ?", "deleting", "archived", "api")
		}
		return query.Updates(updates).Error
	})
}

// discardEvent terminates a stale event which is no longer allowed to mutate
// Engine. Retrying an apply after delete/archive would violate API authority.
func (s *ResourceGateway) discardEvent(e *model.OutboxEvent, cause error) error {
	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var claimed model.OutboxEvent
		if err := tx.Where("id = ? AND claim_token = ? AND processed_at IS NULL", e.ID, e.ClaimToken).First(&claimed).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		claimed.ProcessedAt = &now
		claimed.LastError = cause.Error()
		claimed.ClaimToken = ""
		claimed.ClaimUntil = nil
		if err := tx.Save(&claimed).Error; err != nil {
			return err
		}
		return tx.Model(&model.Operation{}).Where("id = ? AND state NOT IN ?", claimed.OperationID, []string{"succeeded", "failed"}).Updates(map[string]any{"state": "failed", "error": cause.Error()}).Error
	})
}
func (s *ResourceGateway) failEvent(e *model.OutboxEvent, cause error) error {
	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		var claimed model.OutboxEvent
		if err := tx.Where("id = ? AND claim_token = ? AND processed_at IS NULL", e.ID, e.ClaimToken).First(&claimed).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		var op model.Operation
		if err := tx.First(&op, "id = ?", claimed.OperationID).Error; err != nil {
			return err
		}
		claimed.Attempts++
		claimed.LastError = cause.Error()
		op.Error = cause.Error()
		if claimed.Attempts >= maxOutboxAttempts {
			claimed.ProcessedAt = &now
			op.State = "failed"
		} else {
			claimed.AvailableAt = now.Add(time.Duration(claimed.Attempts) * time.Second)
			op.State = "retrying"
		}
		claimed.ClaimToken = ""
		claimed.ClaimUntil = nil
		if err := tx.Save(&claimed).Error; err != nil {
			return err
		}
		return tx.Model(&model.Operation{}).Where("id = ? AND state NOT IN ?", op.ID, []string{"succeeded", "failed"}).Updates(map[string]any{"state": op.State, "error": op.Error}).Error
	})
}
func (s *ResourceGateway) project(ctx context.Context, r *model.ResourceRecord, action string) error {
	if action == "adopt" {
		var c storagev1alpha1.ManagedCluster
		if err := s.k8s.Get(ctx, client.ObjectKey{Name: r.EngineName}, &c); err != nil {
			return err
		}
		enginelabels.AddManagedBy(&c)
		return s.k8s.Update(ctx, &c)
	}
	switch r.Kind {
	case model.ResourceWorkspace:
		var obj workspacev1alpha1.Workspace
		obj.Name = r.EngineName
		if action == "delete" {
			return client.IgnoreNotFound(s.k8s.Delete(ctx, &obj))
		}
		var desired dto.DesiredState
		if err := json.Unmarshal([]byte(r.DesiredJSON), &desired); err != nil {
			return fmt.Errorf("decode workspace desired state: %w", err)
		}
		if desired.Workspace == nil {
			return errors.New("decode workspace desired state: workspace is missing")
		}
		obj.Spec = workspaceSpec(*desired.Workspace)
		obj.Labels = s.businessLabels(r)
		var current workspacev1alpha1.Workspace
		err := s.k8s.Get(ctx, client.ObjectKey{Name: obj.Name}, &current)
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		if err == nil {
			if reflect.DeepEqual(current.Spec, obj.Spec) && reflect.DeepEqual(current.Labels, obj.Labels) {
				return nil
			}
			current.Spec = obj.Spec
			current.Labels = obj.Labels
			return s.k8s.Update(ctx, &current)
		}
		return s.k8s.Create(ctx, &obj)
	case model.ResourceApplication:
		var obj appsv1alpha1.Application
		obj.Name = r.EngineName
		obj.Namespace = r.EngineNamespace
		if action == "delete" {
			return client.IgnoreNotFound(s.k8s.Delete(ctx, &obj))
		}
		var desired dto.DesiredState
		if err := json.Unmarshal([]byte(r.DesiredJSON), &desired); err != nil {
			return fmt.Errorf("decode application desired state: %w", err)
		}
		if desired.Application == nil {
			return errors.New("decode application desired state: application is missing")
		}
		spec, err := applicationSpec(*desired.Application)
		if err != nil {
			return fmt.Errorf("adapt application desired state: %w", err)
		}
		obj.Spec = spec
		obj.Labels = s.businessLabels(r)
		var current appsv1alpha1.Application
		err = s.k8s.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: obj.Namespace}, &current)
		if client.IgnoreNotFound(err) != nil {
			return err
		}
		if err == nil {
			if reflect.DeepEqual(current.Spec, obj.Spec) && reflect.DeepEqual(current.Labels, obj.Labels) {
				return nil
			}
			current.Spec = obj.Spec
			current.Labels = obj.Labels
			return s.k8s.Update(ctx, &current)
		}
		return s.k8s.Create(ctx, &obj)
	}
	return nil
}
func (s *ResourceGateway) businessLabels(r *model.ResourceRecord) map[string]string {
	m := map[string]string{enginelabels.ManagedByKey: enginelabels.ManagedByValue}
	if r.Kind == model.ResourceApplication {
		m[enginelabels.ApplicationIDKey] = r.ID
		m[enginelabels.WorkspaceIDKey] = r.ParentID
	}
	if r.Kind == model.ResourceWorkspace {
		m[enginelabels.WorkspaceIDKey] = r.ID
	}
	if r.ProjectPublicID != "" {
		m[enginelabels.ProjectIDKey] = r.ProjectPublicID
	}
	if r.ModulePublicID != "" {
		m[enginelabels.ModuleIDKey] = r.ModulePublicID
	}
	return m
}

const maxOutboxAttempts = 3

func hasDesired(in dto.DesiredState) bool {
	return in.Workspace != nil || in.Application != nil
}

func validateDesired(kind string, in dto.DesiredState, optional bool) error {
	if optional && !hasDesired(in) {
		return nil
	}
	switch kind {
	case model.ResourceWorkspace:
		if in.Workspace == nil || in.Application != nil {
			return errors.New("desired.workspace is required and must be the only desired state")
		}
	case model.ResourceApplication:
		if in.Application == nil || in.Workspace != nil {
			return errors.New("desired.application is required and must be the only desired state")
		}
		for _, container := range in.Application.Template.Containers {
			if err := validateEnvironment(container.Env); err != nil {
				return err
			}
		}
		for _, override := range in.Application.Overrides {
			if err := validateEnvironment(override.Env); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported resource kind %q", kind)
	}
	return nil
}

func validateEnvironment(envs []dto.EnvironmentVariable) error {
	for _, env := range envs {
		if env.Value != "" {
			return fmt.Errorf("inline environment value for %s is forbidden; use valueFrom.secretName/key", env.Name)
		}
		if env.ValueFrom == nil || env.ValueFrom.SecretName == "" || env.ValueFrom.Key == "" {
			return fmt.Errorf("environment %s requires valueFrom.secretName and key", env.Name)
		}
	}
	return nil
}

func pointer[T any](value T) *T { return &value }

func effectiveWorkspaceNamespace(record model.ResourceRecord) (string, error) {
	var desired dto.DesiredState
	if err := json.Unmarshal([]byte(record.DesiredJSON), &desired); err != nil {
		return "", fmt.Errorf("decode workspace desired state: %w", err)
	}
	if desired.Workspace == nil {
		return "", errors.New("decode workspace desired state: workspace is missing")
	}
	if desired.Workspace.Namespace != "" {
		return desired.Workspace.Namespace, nil
	}
	return record.EngineName, nil
}
