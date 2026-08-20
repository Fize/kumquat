package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fize/kumquat/api/internal/testdb"
	"github.com/fize/kumquat/api/pkg/dto"
	"github.com/fize/kumquat/api/pkg/model"
	appsv1alpha1 "github.com/fize/kumquat/engine/pkg/apis/apps/v1alpha1"
	storagev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/storage/v1alpha1"
	workspacev1alpha1 "github.com/fize/kumquat/engine/pkg/apis/workspace/v1alpha1"
	"github.com/fize/kumquat/engine/pkg/constants"
	enginelabels "github.com/fize/kumquat/engine/pkg/util/labels"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func createOwnershipFixture(t *testing.T, db *gorm.DB, moduleName, projectName string) (model.Module, model.Project) {
	t.Helper()
	m := model.Module{Name: moduleName}
	requireNoError(t, db.Create(&m).Error)
	p := model.Project{Name: projectName, ModuleID: m.ID}
	requireNoError(t, db.Omit("config").Create(&p).Error)
	return m, p
}

func TestResourceGatewayScopesResourcesAndOperationsToActorModule(t *testing.T) {
	db := resourceDB(t)
	moduleA, projectA := createOwnershipFixture(t, db, "team-a", "project-a")
	_, projectB := createOwnershipFixture(t, db, "team-b", "project-b")
	g := NewResourceGateway(db, nil)
	actorA := ContextWithPrincipal(context.Background(), Principal{UserID: 11, ModulePublicID: moduleA.PublicID})
	desired := dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "tenant-a"}}
	op, err := g.Create(actorA, model.ResourceWorkspace, ResourceInput{Name: "a", ProjectID: projectA.PublicID, Desired: desired}, "create")
	requireNoError(t, err)
	if _, err := g.Create(actorA, model.ResourceWorkspace, ResourceInput{Name: "b", ProjectID: projectB.PublicID, Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "tenant-b"}}}, "cross"); err == nil {
		t.Fatal("cross-module parent accepted")
	}
	other := ContextWithPrincipal(context.Background(), Principal{UserID: 22, ModulePublicID: "module_other"})
	if _, err := g.Get(other, op.ResourceID, model.ResourceWorkspace); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-scope get=%v", err)
	}
	if _, err := g.Operation(other, op.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-scope operation=%v", err)
	}
}

func TestResourceGatewayRepairsDeletedProjectionAndArchivesForRecreation(t *testing.T) {
	db := resourceDB(t)
	_, project := createOwnershipFixture(t, db, "team", "project")
	scheme := runtime.NewScheme()
	requireNoError(t, workspacev1alpha1.AddToScheme(scheme))
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
	g := NewResourceGateway(db, k8s)
	ctx := context.Background()
	in := ResourceInput{Name: "workspace", ProjectID: project.PublicID, Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "tenant"}}}
	op, err := g.Create(ctx, model.ResourceWorkspace, in, "create")
	requireNoError(t, err)
	requireNoError(t, g.ReconcileOne(ctx))
	var engine workspacev1alpha1.Workspace
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: "workspace"}, &engine))
	requireNoError(t, k8s.Delete(ctx, &engine))
	requireNoError(t, g.SyncObserved(ctx))
	requireNoError(t, g.ReconcileOne(ctx))
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: "workspace"}, &engine))
	engine.Spec.Name = "drifted"
	requireNoError(t, k8s.Update(ctx, &engine))
	requireNoError(t, g.EnsureDesired(ctx))
	requireNoError(t, g.ReconcileOne(ctx))
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: "workspace"}, &engine))
	if engine.Spec.Name != "tenant" {
		t.Fatalf("drift not repaired: %s", engine.Spec.Name)
	}
	_, err = g.Delete(ctx, op.ResourceID, model.ResourceWorkspace, "delete")
	requireNoError(t, err)
	requireNoError(t, g.ReconcileOne(ctx))
	if _, err := g.Get(ctx, op.ResourceID, model.ResourceWorkspace); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("archived resource visible: %v", err)
	}
	if _, err := g.Create(ctx, model.ResourceWorkspace, in, "recreate"); err != nil {
		t.Fatalf("recreate after archive: %v", err)
	}
}

func TestResourceGatewayIdempotencyFingerprintAndOutboxClaim(t *testing.T) {
	db := resourceDB(t)
	_, project := createOwnershipFixture(t, db, "team", "project")
	scheme := runtime.NewScheme()
	requireNoError(t, workspacev1alpha1.AddToScheme(scheme))
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
	g := NewResourceGateway(db, k8s)
	ctx := ContextWithPrincipal(context.Background(), Principal{UserID: 7, Admin: true})
	in := ResourceInput{Name: "workspace", ProjectID: project.PublicID, Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "tenant"}}}
	op, err := g.Create(ctx, model.ResourceWorkspace, in, "same")
	requireNoError(t, err)
	mismatch := in
	mismatch.Desired.Workspace.Namespace = "other"
	if _, err := g.Create(ctx, model.ResourceWorkspace, mismatch, "same"); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("mismatch=%v", err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { defer wg.Done(); errs <- g.ReconcileOne(context.Background()) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !strings.Contains(err.Error(), "locked") {
			t.Fatal(err)
		}
	}
	var event model.OutboxEvent
	requireNoError(t, db.First(&event, "operation_id = ?", op.ID).Error)
	if event.ProcessedAt == nil {
		t.Fatal("event was not completed")
	}
}

func TestResourceGatewayConcurrentIdempotencyReturnsWinningOperation(t *testing.T) {
	db := resourceDB(t)
	sqlDB, err := db.DB()
	requireNoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	_, project := createOwnershipFixture(t, db, "team", "project")
	g := NewResourceGateway(db, nil)
	ctx := ContextWithPrincipal(context.Background(), Principal{UserID: 9, Admin: true})
	in := ResourceInput{Name: "workspace", ProjectID: project.PublicID, Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "tenant"}}}
	type result struct {
		op  *model.Operation
		err error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			op, err := g.Create(ctx, model.ResourceWorkspace, in, "concurrent")
			results <- result{op, err}
		}()
	}
	wg.Wait()
	close(results)
	var winner string
	for got := range results {
		requireNoError(t, got.err)
		if got.op == nil {
			t.Fatal("nil operation")
		}
		if winner == "" {
			winner = got.op.ID
		} else if winner != got.op.ID {
			t.Fatalf("operations differ: %s %s", winner, got.op.ID)
		}
	}
	var operations, events int64
	requireNoError(t, db.Model(&model.Operation{}).Count(&operations).Error)
	requireNoError(t, db.Model(&model.OutboxEvent{}).Count(&events).Error)
	if operations != 1 || events != 1 {
		t.Fatalf("operations/events=%d/%d", operations, events)
	}
}

func TestResourceGatewayRejectsAndRedactsInlineEnvironmentValues(t *testing.T) {
	db := resourceDB(t)
	g := NewResourceGateway(db, nil)
	desired := dto.DesiredState{Application: &dto.ApplicationDesired{Workload: dto.WorkloadRef{APIVersion: "apps/v1", Kind: "Deployment"}, Template: dto.PodTemplate{Containers: []dto.Container{{Name: "app", Image: "nginx", Env: []dto.EnvironmentVariable{{Name: "TOKEN", Value: "secret"}}}}}}}
	if err := validateDesired(model.ResourceApplication, desired, false); err == nil {
		t.Fatal("inline secret accepted")
	}
	desired.Application.Template.Containers[0].Env[0] = dto.EnvironmentVariable{Name: "TOKEN", ValueFrom: &dto.SecretKeyRef{SecretName: "app", Key: "token"}}
	if err := validateDesired(model.ResourceApplication, desired, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(desired)
	view, err := recordView(model.ResourceRecord{DesiredJSON: string(raw)})
	requireNoError(t, err)
	if view.Desired.Application.Template.Containers[0].Env[0].Value != "" {
		t.Fatal("inline value returned")
	}
	_ = g
}

func resourceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testdb.Open(t)
	if err := db.AutoMigrate(&model.Module{}, &model.Project{}, &model.ResourceRecord{}, &model.Operation{}, &model.OutboxEvent{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDiscoverClustersDoesNotPersistCredentialAnnotations(t *testing.T) {
	db := resourceDB(t)
	scheme := runtime.NewScheme()
	if err := storagev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &storagev1alpha1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "edge-1", Labels: map[string]string{constants.LabelRegistrationSource: constants.RegistrationSourceAgent}, Annotations: map[string]string{"kumquat.io/token": "top-secret"}}, Spec: storagev1alpha1.ManagedClusterSpec{ConnectionMode: storagev1alpha1.ClusterConnectionModeEdge}}
	unlabeledEdge := &storagev1alpha1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "edge-unlabeled"}, Spec: storagev1alpha1.ManagedClusterSpec{ConnectionMode: storagev1alpha1.ClusterConnectionModeEdge}}
	hub := &storagev1alpha1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "hub", Labels: map[string]string{constants.LabelRegistrationSource: constants.RegistrationSourceAgent}}, Spec: storagev1alpha1.ManagedClusterSpec{ConnectionMode: storagev1alpha1.ClusterConnectionModeHub}}
	g := NewResourceGateway(db, fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, unlabeledEdge, hub).Build())
	if err := g.DiscoverClusters(context.Background()); err != nil {
		t.Fatal(err)
	}
	var record model.ResourceRecord
	if err := db.Where("kind = ?", model.ResourceCluster).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.Source != "agent" || record.State != "discovered" {
		t.Fatalf("source/state=%s/%s", record.Source, record.State)
	}
	if strings.Contains(record.ObservedJSON, "top-secret") || strings.Contains(record.ObservedJSON, "annotations") {
		t.Fatalf("credential metadata leaked: %s", record.ObservedJSON)
	}
	var count int64
	requireNoError(t, db.Model(&model.ResourceRecord{}).Where("kind = ?", model.ResourceCluster).Count(&count).Error)
	if count != 1 {
		t.Fatalf("discovered cluster count = %d, want 1", count)
	}
}

func TestDiscoverAndAdoptExistingWorkspaceAndApplicationWithoutTransfer(t *testing.T) {
	db := resourceDB(t)
	module, project := createOwnershipFixture(t, db, "team", "project")
	scheme := runtime.NewScheme()
	requireNoError(t, workspacev1alpha1.AddToScheme(scheme))
	requireNoError(t, appsv1alpha1.AddToScheme(scheme))
	template, _ := json.Marshal(map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "app", "image": "nginx"}}}})
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "legacy"}, Spec: workspacev1alpha1.WorkspaceSpec{Name: "tenant", ClusterSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"region": "east"}}}}
	application := &appsv1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "legacy-app", Namespace: "tenant"}, Spec: appsv1alpha1.ApplicationSpec{Workload: appsv1alpha1.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}, Template: runtime.RawExtension{Raw: template}}}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, application).Build()
	g := NewResourceGateway(db, k8s)
	ctx := context.Background()
	requireNoError(t, g.DiscoverEngineResources(ctx))
	var ws, app model.ResourceRecord
	requireNoError(t, db.Where("kind = ? AND engine_name = ?", model.ResourceWorkspace, "legacy").First(&ws).Error)
	requireNoError(t, db.Where("kind = ? AND engine_name = ?", model.ResourceApplication, "legacy-app").First(&app).Error)
	if ws.State != "unassigned" || app.State != "unassigned" || ws.ModulePublicID != "" || app.ParentID != "" {
		t.Fatalf("invented ownership: %#v %#v", ws, app)
	}
	_, err := g.AdoptExisting(ctx, ws.ID, model.ResourceWorkspace, ResourceInput{ProjectID: project.PublicID}, "adopt-ws")
	requireNoError(t, err)
	requireNoError(t, g.ReconcileOne(ctx))
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: workspace.Name}, workspace))
	if workspace.Spec.ClusterSelector == nil || workspace.Spec.ClusterSelector.MatchLabels["region"] != "east" {
		t.Fatalf("workspace adoption lost selector: %#v", workspace.Spec)
	}
	_, err = g.AdoptExisting(ctx, app.ID, model.ResourceApplication, ResourceInput{WorkspaceID: ws.ID}, "adopt-app")
	requireNoError(t, err)
	requireNoError(t, g.ReconcileOne(ctx))
	requireNoError(t, db.First(&app, "id = ?", app.ID).Error)
	if app.State != "adopted" || app.Source != "api" || app.ModulePublicID != module.PublicID || app.ParentID != ws.ID {
		t.Fatalf("adoption=%#v", app)
	}
	requireNoError(t, k8s.Delete(ctx, application))
	requireNoError(t, g.EnsureDesired(ctx))
	for i := 0; i < 3; i++ {
		requireNoError(t, g.ReconcileOne(ctx))
	}
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: application.Name, Namespace: application.Namespace}, application))
	application.Spec.Replicas = pointer(int32(9))
	requireNoError(t, k8s.Update(ctx, application))
	requireNoError(t, g.EnsureDesired(ctx))
	for i := 0; i < 3; i++ {
		requireNoError(t, g.ReconcileOne(ctx))
	}
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: application.Name, Namespace: application.Namespace}, application))
	if application.Spec.Replicas != nil {
		t.Fatalf("adopted drift was not repaired: %#v", application.Spec.Replicas)
	}
	if _, err := g.AdoptExisting(ctx, app.ID, model.ResourceApplication, ResourceInput{WorkspaceID: ws.ID}, "second-adopt"); err == nil {
		t.Fatal("already adopted application was reassigned")
	}
}

func TestAdoptExistingApplicationRejectsDeletingAndArchivedWorkspace(t *testing.T) {
	db := resourceDB(t)
	_, project := createOwnershipFixture(t, db, "team", "project")
	scheme := runtime.NewScheme()
	requireNoError(t, workspacev1alpha1.AddToScheme(scheme))
	requireNoError(t, appsv1alpha1.AddToScheme(scheme))
	template, _ := json.Marshal(map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "app", "image": "nginx"}}}})
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "legacy"}, Spec: workspacev1alpha1.WorkspaceSpec{Name: "tenant"}}
	application := &appsv1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "legacy-app", Namespace: "tenant"}, Spec: appsv1alpha1.ApplicationSpec{Workload: appsv1alpha1.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}, Template: runtime.RawExtension{Raw: template}}}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, application).Build()
	g := NewResourceGateway(db, k8s)
	ctx := context.Background()
	requireNoError(t, g.DiscoverEngineResources(ctx))
	var ws, app model.ResourceRecord
	requireNoError(t, db.Where("kind = ? AND engine_name = ?", model.ResourceWorkspace, "legacy").First(&ws).Error)
	requireNoError(t, db.Where("kind = ? AND engine_name = ?", model.ResourceApplication, "legacy-app").First(&app).Error)
	_, err := g.AdoptExisting(ctx, ws.ID, model.ResourceWorkspace, ResourceInput{ProjectID: project.PublicID}, "adopt-ws-before-delete")
	requireNoError(t, err)
	requireNoError(t, g.ReconcileOne(ctx))
	_, err = g.Delete(ctx, ws.ID, model.ResourceWorkspace, "delete-ws-before-adopt")
	requireNoError(t, err)
	if _, err = g.AdoptExisting(ctx, app.ID, model.ResourceApplication, ResourceInput{WorkspaceID: ws.ID}, "adopt-app-while-deleting"); !errors.Is(err, ErrParentUnavailable) {
		t.Fatalf("adopt under deleting workspace = %v, want %v", err, ErrParentUnavailable)
	}
	requireNoError(t, g.ReconcileOne(ctx))
	if _, err = g.AdoptExisting(ctx, app.ID, model.ResourceApplication, ResourceInput{WorkspaceID: ws.ID}, "adopt-app-after-archive"); !errors.Is(err, ErrParentUnavailable) {
		t.Fatalf("adopt under archived workspace = %v, want %v", err, ErrParentUnavailable)
	}
	requireNoError(t, db.First(&app, "id = ?", app.ID).Error)
	if app.State != "unassigned" || app.Source != "engine" || app.ParentID != "" {
		t.Fatalf("failed adoption mutated application: %#v", app)
	}
}

func TestResourceGatewayDeletingAndRevisionGuardsPreventStaleApply(t *testing.T) {
	db := resourceDB(t)
	_, project := createOwnershipFixture(t, db, "team", "project")
	scheme := runtime.NewScheme()
	requireNoError(t, workspacev1alpha1.AddToScheme(scheme))
	requireNoError(t, appsv1alpha1.AddToScheme(scheme))
	k8s := fake.NewClientBuilder().WithScheme(scheme).Build()
	g := NewResourceGateway(db, k8s)
	ctx := context.Background()
	op, err := g.Create(ctx, model.ResourceWorkspace, ResourceInput{Name: "doomed", ProjectID: project.PublicID, Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "doomed"}}}, "create-doomed")
	requireNoError(t, err)
	_, err = g.Delete(ctx, op.ResourceID, model.ResourceWorkspace, "delete-doomed")
	requireNoError(t, err)
	if _, err := g.Update(ctx, op.ResourceID, model.ResourceWorkspace, ResourceInput{Name: "doomed", Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "doomed"}}}, "update-deleting"); err == nil {
		t.Fatal("update accepted while deleting")
	}
	applicationDesired := dto.DesiredState{Application: &dto.ApplicationDesired{Workload: dto.WorkloadRef{APIVersion: "apps/v1", Kind: "Deployment"}, Template: dto.PodTemplate{Containers: []dto.Container{{Name: "app", Image: "nginx"}}}}}
	if _, err := g.Create(ctx, model.ResourceApplication, ResourceInput{Name: "child", WorkspaceID: op.ResourceID, Desired: applicationDesired}, "child-deleting"); err == nil {
		t.Fatal("child accepted under deleting workspace")
	}
	requireNoError(t, g.ReconcileOne(ctx)) // stale apply is discarded
	var engine workspacev1alpha1.Workspace
	if err := k8s.Get(ctx, client.ObjectKey{Name: "doomed"}, &engine); !apierrors.IsNotFound(err) {
		t.Fatalf("stale apply projected archived resource: %v", err)
	}
	requireNoError(t, g.ReconcileOne(ctx)) // delete archives
	if err := k8s.Get(ctx, client.ObjectKey{Name: "doomed"}, &engine); !apierrors.IsNotFound(err) {
		t.Fatalf("deleted resource exists: %v", err)
	}

	op2, err := g.Create(ctx, model.ResourceWorkspace, ResourceInput{Name: "revised", ProjectID: project.PublicID, Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "v1"}}}, "create-revised")
	requireNoError(t, err)
	_, err = g.Update(ctx, op2.ResourceID, model.ResourceWorkspace, ResourceInput{Name: "revised", Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "v2"}}}, "update-revised")
	requireNoError(t, err)
	requireNoError(t, g.ReconcileOne(ctx))
	requireNoError(t, g.ReconcileOne(ctx))
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: "revised"}, &engine))
	if engine.Spec.Name != "v2" {
		t.Fatalf("stale revision won: %s", engine.Spec.Name)
	}
}

func TestOverrideEnvironmentIsSecretOnlyPersistedAndRedacted(t *testing.T) {
	db := resourceDB(t)
	_, project := createOwnershipFixture(t, db, "team", "project")
	g := NewResourceGateway(db, nil)
	ctx := context.Background()
	ws, err := g.Create(ctx, model.ResourceWorkspace, ResourceInput{Name: "workspace", ProjectID: project.PublicID, Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "tenant"}}}, "ws")
	requireNoError(t, err)
	base := dto.ApplicationDesired{Workload: dto.WorkloadRef{APIVersion: "apps/v1", Kind: "Deployment"}, Template: dto.PodTemplate{Containers: []dto.Container{{Name: "app", Image: "nginx"}}}, Overrides: []dto.PolicyOverride{{Env: []dto.EnvironmentVariable{{Name: "TOKEN", Value: "plaintext"}}}}}
	if _, err := g.Create(ctx, model.ResourceApplication, ResourceInput{Name: "bad", WorkspaceID: ws.ResourceID, Desired: dto.DesiredState{Application: &base}}, "bad"); err == nil {
		t.Fatal("inline override environment accepted")
	}
	base.Overrides[0].Env[0] = dto.EnvironmentVariable{Name: "TOKEN", ValueFrom: &dto.SecretKeyRef{SecretName: "credentials", Key: "token"}}
	op, err := g.Create(ctx, model.ResourceApplication, ResourceInput{Name: "good", WorkspaceID: ws.ResourceID, Desired: dto.DesiredState{Application: &base}}, "good")
	requireNoError(t, err)
	var record model.ResourceRecord
	requireNoError(t, db.First(&record, "id = ?", op.ResourceID).Error)
	if strings.Contains(record.DesiredJSON, "plaintext") || !strings.Contains(record.DesiredJSON, "credentials") {
		t.Fatalf("unsafe persistence: %s", record.DesiredJSON)
	}
	view, err := recordView(record)
	requireNoError(t, err)
	got := view.Desired.Application.Overrides[0].Env[0]
	if got.Value != "" || got.ValueFrom == nil || got.ValueFrom.SecretName != "credentials" || got.ValueFrom.Key != "token" {
		t.Fatalf("override round trip=%#v", got)
	}
}

func TestUnsupportedImportedApplicationIsDiscoverableButCannotBecomeAuthoritative(t *testing.T) {
	db := resourceDB(t)
	_, project := createOwnershipFixture(t, db, "team", "project")
	scheme := runtime.NewScheme()
	requireNoError(t, workspacev1alpha1.AddToScheme(scheme))
	requireNoError(t, appsv1alpha1.AddToScheme(scheme))
	template, _ := json.Marshal(corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx", Ports: []corev1.ContainerPort{{ContainerPort: 80}}}}}})
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "legacy"}, Spec: workspacev1alpha1.WorkspaceSpec{Name: "tenant"}}
	application := &appsv1alpha1.Application{ObjectMeta: metav1.ObjectMeta{Name: "unsupported", Namespace: "tenant"}, Spec: appsv1alpha1.ApplicationSpec{Workload: appsv1alpha1.WorkloadGVK{APIVersion: "apps/v1", Kind: "Deployment"}, Template: runtime.RawExtension{Raw: template}}}
	g := NewResourceGateway(db, fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, application).Build())
	ctx := context.Background()
	requireNoError(t, g.DiscoverEngineResources(ctx))
	var ws, app model.ResourceRecord
	requireNoError(t, db.Where("kind = ?", model.ResourceWorkspace).First(&ws).Error)
	requireNoError(t, db.Where("kind = ?", model.ResourceApplication).First(&app).Error)
	if app.SyncState != "unsupported" || app.CapabilityError == "" {
		t.Fatalf("unsupported import=%#v", app)
	}
	_, err := g.AdoptExisting(ctx, ws.ID, model.ResourceWorkspace, ResourceInput{ProjectID: project.PublicID}, "adopt-ws")
	requireNoError(t, err)
	requireNoError(t, g.ReconcileOne(ctx))
	if _, err := g.AdoptExisting(ctx, app.ID, model.ResourceApplication, ResourceInput{WorkspaceID: ws.ID}, "adopt-unsupported"); err == nil || !strings.Contains(err.Error(), "cannot be adopted without loss") {
		t.Fatalf("unsupported adoption error=%v", err)
	}
	requireNoError(t, db.First(&app, "id = ?", app.ID).Error)
	if app.Source != "engine" || app.State != "unassigned" {
		t.Fatalf("unsupported app became authoritative: %#v", app)
	}
}

func TestResourceGatewayTransactionIdempotencyAndImmutableParent(t *testing.T) {
	db := resourceDB(t)
	module := model.Module{Name: "team"}
	if err := db.Create(&module).Error; err != nil {
		t.Fatal(err)
	}
	project := model.Project{Name: "project", ModuleID: module.ID}
	if err := db.Omit("config").Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	g := NewResourceGateway(db, nil)
	ctx := context.Background()
	projectID := project.PublicID
	desired := dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "workspace"}}
	ws, err := g.Create(ctx, model.ResourceWorkspace, ResourceInput{Name: "workspace", ProjectID: projectID, Desired: desired}, "create-ws")
	if err != nil {
		t.Fatal(err)
	}
	again, err := g.Create(ctx, model.ResourceWorkspace, ResourceInput{Name: "workspace", ProjectID: projectID, Desired: desired}, "create-ws")
	if err != nil {
		t.Fatal(err)
	}
	if ws.ID != again.ID {
		t.Fatalf("idempotency returned %s, want %s", again.ID, ws.ID)
	}
	var record model.ResourceRecord
	if err := db.First(&record, "id = ?", ws.ResourceID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := g.Update(ctx, record.ID, model.ResourceWorkspace, ResourceInput{Name: record.Name, ProjectID: "2"}, "move"); !errors.Is(err, ErrParentImmutable) {
		t.Fatalf("expected immutable parent conflict, got %v", err)
	}
	var events int64
	db.Model(&model.OutboxEvent{}).Count(&events)
	if events != 1 {
		t.Fatalf("events=%d", events)
	}
}

func TestResourceGatewayProjectionLifecycleObservedAndClusterAdoption(t *testing.T) {
	db := resourceDB(t)
	module := model.Module{Name: "team"}
	requireNoError(t, db.Create(&module).Error)
	project := model.Project{Name: "project", ModuleID: module.ID}
	requireNoError(t, db.Omit("config").Create(&project).Error)

	scheme := runtime.NewScheme()
	requireNoError(t, appsv1alpha1.AddToScheme(scheme))
	requireNoError(t, workspacev1alpha1.AddToScheme(scheme))
	requireNoError(t, storagev1alpha1.AddToScheme(scheme))
	edge := &storagev1alpha1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "edge", Labels: map[string]string{constants.LabelRegistrationSource: constants.RegistrationSourceAgent}}, Spec: storagev1alpha1.ManagedClusterSpec{ConnectionMode: storagev1alpha1.ClusterConnectionModeEdge}}
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithObjects(edge).WithStatusSubresource(&workspacev1alpha1.Workspace{}, &appsv1alpha1.Application{}).Build()
	gateway := NewResourceGateway(db, k8s)
	ctx := context.Background()

	workspaceDesired := dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "tenant-a"}}
	workspaceOp, err := gateway.Create(ctx, model.ResourceWorkspace, ResourceInput{Name: "workspace", ProjectID: project.PublicID, Desired: workspaceDesired}, "workspace-create")
	requireNoError(t, err)
	requireNoError(t, gateway.ReconcileOne(ctx))
	var workspace workspacev1alpha1.Workspace
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: "workspace"}, &workspace))
	if workspace.Labels[enginelabels.ProjectIDKey] != project.PublicID || workspace.Labels[enginelabels.ModuleIDKey] != module.PublicID {
		t.Fatalf("workspace ownership labels = %#v", workspace.Labels)
	}
	workspace.Status.Conditions = []metav1.Condition{{Type: "Ready", Status: metav1.ConditionTrue}}
	workspace.Status.AppliedClusters = []string{"edge"}
	requireNoError(t, k8s.Status().Update(ctx, &workspace))
	requireNoError(t, gateway.SyncObserved(ctx))
	workspaceView, err := gateway.Get(ctx, workspaceOp.ResourceID, model.ResourceWorkspace)
	requireNoError(t, err)
	if workspaceView.ObservedAt == nil || workspaceView.Observed.Workspace == nil || !workspaceView.Observed.Workspace.Ready {
		t.Fatalf("workspace observed projection is not fresh: %#v", workspaceView)
	}

	workspaceDesired.Workspace.Namespace = "tenant-b"
	_, err = gateway.Update(ctx, workspaceOp.ResourceID, model.ResourceWorkspace, ResourceInput{Name: "workspace", ProjectID: project.PublicID, Desired: workspaceDesired}, "workspace-update")
	requireNoError(t, err)
	requireNoError(t, gateway.ReconcileOne(ctx))
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: "workspace"}, &workspace))
	if workspace.Spec.Name != "tenant-b" {
		t.Fatalf("workspace namespace = %q", workspace.Spec.Name)
	}

	applicationDesired := dto.DesiredState{Application: &dto.ApplicationDesired{Workload: dto.WorkloadRef{APIVersion: "apps/v1", Kind: "Deployment"}, Template: dto.PodTemplate{Containers: []dto.Container{{Name: "app", Image: "nginx"}}}}}
	applicationOp, err := gateway.Create(ctx, model.ResourceApplication, ResourceInput{Name: "application", WorkspaceID: workspaceOp.ResourceID, Desired: applicationDesired}, "application-create")
	requireNoError(t, err)
	requireNoError(t, gateway.ReconcileOne(ctx))
	var application appsv1alpha1.Application
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: "application", Namespace: "tenant-b"}, &application))
	if application.Labels[enginelabels.ProjectIDKey] != project.PublicID || application.Labels[enginelabels.ModuleIDKey] != module.PublicID {
		t.Fatalf("application ownership labels = %#v", application.Labels)
	}
	changedNamespace := dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "tenant-c"}}
	if _, err := gateway.Update(ctx, workspaceOp.ResourceID, model.ResourceWorkspace, ResourceInput{Name: "workspace", ProjectID: project.PublicID, Desired: changedNamespace}, "workspace-namespace-in-use"); !errors.Is(err, ErrWorkspaceNamespaceInUse) {
		t.Fatalf("workspace namespace update with children error = %v", err)
	}
	_, err = gateway.Delete(ctx, applicationOp.ResourceID, model.ResourceApplication, "application-delete")
	requireNoError(t, err)
	requireNoError(t, gateway.ReconcileOne(ctx))
	if err := k8s.Get(ctx, client.ObjectKey{Name: "application", Namespace: "tenant-b"}, &application); client.IgnoreNotFound(err) != nil || err == nil {
		t.Fatalf("application delete error = %v", err)
	}

	requireNoError(t, gateway.DiscoverClusters(ctx))
	var clusterRecord model.ResourceRecord
	requireNoError(t, db.Where("kind = ? AND engine_name = ?", model.ResourceCluster, "edge").First(&clusterRecord).Error)
	_, err = gateway.AdoptCluster(ctx, clusterRecord.ID, "cluster-adopt")
	requireNoError(t, err)
	requireNoError(t, gateway.ReconcileOne(ctx))
	requireNoError(t, k8s.Get(ctx, client.ObjectKey{Name: "edge"}, edge))
	if edge.Labels[enginelabels.ManagedByKey] != enginelabels.ManagedByValue {
		t.Fatalf("cluster managed-by label = %q", edge.Labels[enginelabels.ManagedByKey])
	}
}

func TestResourceGatewayRetryStateIsNonTerminalUntilExhausted(t *testing.T) {
	db := resourceDB(t)
	module := model.Module{Name: "team"}
	requireNoError(t, db.Create(&module).Error)
	project := model.Project{Name: "project", ModuleID: module.ID}
	requireNoError(t, db.Omit("config").Create(&project).Error)
	scheme := runtime.NewScheme()
	requireNoError(t, workspacev1alpha1.AddToScheme(scheme))
	k8s := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
		return fmt.Errorf("temporary engine failure")
	}}).Build()
	gateway := NewResourceGateway(db, k8s)
	op, err := gateway.Create(context.Background(), model.ResourceWorkspace, ResourceInput{Name: "workspace", ProjectID: project.PublicID, Desired: dto.DesiredState{Workspace: &dto.WorkspaceDesired{Namespace: "tenant"}}}, "retry")
	requireNoError(t, err)
	for attempt := 1; attempt <= maxOutboxAttempts; attempt++ {
		requireNoError(t, db.Model(&model.OutboxEvent{}).Where("operation_id = ?", op.ID).Update("available_at", time.Now().Add(-time.Second)).Error)
		requireNoError(t, gateway.ReconcileOne(context.Background()))
		requireNoError(t, db.First(op, "id = ?", op.ID).Error)
		if attempt < maxOutboxAttempts && op.State != "retrying" {
			t.Fatalf("attempt %d state = %q", attempt, op.State)
		}
	}
	if op.State != "failed" {
		t.Fatalf("exhausted state = %q", op.State)
	}
	var event model.OutboxEvent
	requireNoError(t, db.First(&event, "operation_id = ?", op.ID).Error)
	if event.ProcessedAt == nil {
		t.Fatal("exhausted event remains retry eligible")
	}
}

func TestResourceGatewayMissingRecordRetryEventuallyFailsOperation(t *testing.T) {
	db := resourceDB(t)
	op := model.Operation{ID: model.NewPublicID("op"), IdempotencyKey: "missing-record", ResourceID: model.NewPublicID("workspace"), Action: "apply", State: "pending"}
	requireNoError(t, db.Create(&op).Error)
	event := model.OutboxEvent{ID: model.NewPublicID("evt"), OperationID: op.ID, ResourceID: op.ResourceID, Action: "apply", AvailableAt: time.Now().Add(-time.Second)}
	requireNoError(t, db.Create(&event).Error)
	gateway := NewResourceGateway(db, nil)
	for attempt := 1; attempt <= maxOutboxAttempts; attempt++ {
		requireNoError(t, db.Model(&event).Update("available_at", time.Now().Add(-time.Second)).Error)
		requireNoError(t, gateway.ReconcileOne(context.Background()))
		requireNoError(t, db.First(&op, "id = ?", op.ID).Error)
		if attempt < maxOutboxAttempts && op.State != "retrying" {
			t.Fatalf("attempt %d state = %q", attempt, op.State)
		}
	}
	if op.State != "failed" {
		t.Fatalf("exhausted missing-record operation state = %q", op.State)
	}
	requireNoError(t, db.First(&event, "id = ?", event.ID).Error)
	if event.ProcessedAt == nil {
		t.Fatal("missing-record event remains retry eligible after exhaustion")
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
