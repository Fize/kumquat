package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/fize/kumquat/apiserver/pkg/model"
	"gorm.io/gorm"
)

// Migrate performs database migration
func Migrate(db *gorm.DB) error {
	// Existing installations did not have public_id. Add it as nullable and
	// backfill before the model's NOT NULL constraint is applied; SQLite cannot
	// add a populated-table NOT NULL column without a default.
	if err := migratePublicIDs(db); err != nil {
		return err
	}
	// Superseded global uniqueness made Application names global and kept
	// tombstoned records reserved. Drop it before installing scoped active keys.
	if db.Migrator().HasIndex(&model.ResourceRecord{}, "idx_resource_kind_name") {
		if err := db.Migrator().DropIndex(&model.ResourceRecord{}, "idx_resource_kind_name"); err != nil {
			return err
		}
	}
	if db.Migrator().HasIndex(&model.Operation{}, "idx_operations_idempotency_key") {
		if err := db.Migrator().DropIndex(&model.Operation{}, "idx_operations_idempotency_key"); err != nil {
			return err
		}
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.Module{},
		&model.Project{},
		&model.ResourceRecord{},
		&model.Operation{},
		&model.OutboxEvent{},
	); err != nil {
		return err
	}
	if err := backfillResourceIdentity(db); err != nil {
		return err
	}
	if err := backfillOutboxRevisions(db); err != nil {
		return err
	}
	if err := enforcePublicIDConstraint(db, &model.Module{}, "modules"); err != nil {
		return err
	}
	return enforcePublicIDConstraint(db, &model.Project{}, "projects")
}

func backfillOutboxRevisions(db *gorm.DB) error {
	return db.Exec("UPDATE outbox_events SET desired_revision = (SELECT desired_revision FROM resource_records WHERE resource_records.id = outbox_events.resource_id) WHERE desired_revision = 0 AND resource_id IN (SELECT id FROM resource_records)").Error
}

func backfillResourceIdentity(db *gorm.DB) error {
	var rows []model.ResourceRecord
	if err := db.Where("archived_at IS NULL AND (active_key IS NULL OR active_key = '' OR desired_hash = '' OR desired_revision = 0)").Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		r := &rows[i]
		namespace := r.EngineNamespace
		if r.Kind == model.ResourceWorkspace && namespace == "" {
			var payload struct {
				Workspace *struct {
					Namespace string `json:"namespace"`
				} `json:"workspace"`
			}
			_ = json.Unmarshal([]byte(r.DesiredJSON), &payload)
			if payload.Workspace != nil {
				namespace = payload.Workspace.Namespace
			}
			if namespace == "" {
				namespace = r.EngineName
			}
		}
		key := r.Kind + ":" + r.EngineName
		if r.Kind == model.ResourceWorkspace {
			key = r.Kind + ":namespace:" + namespace
		}
		if r.Kind == model.ResourceApplication {
			key = r.Kind + ":" + namespace + ":" + r.EngineName
		}
		sum := sha256.Sum256([]byte(r.DesiredJSON))
		hash := hex.EncodeToString(sum[:])
		revision := r.DesiredRevision
		if revision == 0 {
			revision = 1
		}
		if err := db.Model(&model.ResourceRecord{}).Where("id = ?", r.ID).Updates(map[string]interface{}{"active_key": key, "desired_hash": hash, "desired_revision": revision, "engine_namespace": namespace}).Error; err != nil {
			return fmt.Errorf("backfill resource %s identity: %w", r.ID, err)
		}
	}
	return nil
}

type nullableModulePublicID struct {
	PublicID *string `gorm:"column:public_id;size:40"`
}

func (nullableModulePublicID) TableName() string { return "modules" }

type nullableProjectPublicID struct {
	PublicID *string `gorm:"column:public_id;size:40"`
}

func (nullableProjectPublicID) TableName() string { return "projects" }

type legacyIDRow struct {
	ID       uint
	PublicID *string
}

func migratePublicIDs(db *gorm.DB) error {
	if err := migratePublicID(db, &model.Module{}, &nullableModulePublicID{}, "modules", "module"); err != nil {
		return err
	}
	return migratePublicID(db, &model.Project{}, &nullableProjectPublicID{}, "projects", "project")
}

func migratePublicID(db *gorm.DB, constrained, nullable interface{}, table, prefix string) error {
	if !db.Migrator().HasTable(table) {
		return db.AutoMigrate(constrained)
	}
	if !db.Migrator().HasColumn(table, "public_id") {
		if err := db.Migrator().AddColumn(nullable, "PublicID"); err != nil {
			return fmt.Errorf("add nullable %s.public_id: %w", table, err)
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		var rows []legacyIDRow
		if err := tx.Table(table).Select("id", "public_id").Where("public_id IS NULL OR public_id = ''").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if err := tx.Table(table).Where("id = ?", row.ID).Update("public_id", model.NewPublicID(prefix)).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("backfill %s.public_id: %w", table, err)
	}
	if !db.Migrator().HasIndex(constrained, "PublicID") {
		if err := db.Migrator().CreateIndex(constrained, "PublicID"); err != nil {
			return fmt.Errorf("create %s.public_id unique index: %w", table, err)
		}
	}
	return enforcePublicIDConstraint(db, constrained, table)
}

func enforcePublicIDConstraint(db *gorm.DB, constrained interface{}, table string) error {
	if !db.Migrator().HasIndex(constrained, "PublicID") {
		if err := db.Migrator().CreateIndex(constrained, "PublicID"); err != nil {
			return fmt.Errorf("create %s.public_id unique index: %w", table, err)
		}
	}
	if db.Dialector.Name() == "sqlite" {
		// SQLite cannot add a NOT NULL constraint in place. Triggers enforce the
		// same invariant without rebuilding referenced legacy tables.
		for _, action := range []string{"INSERT", "UPDATE OF public_id"} {
			name := fmt.Sprintf("trg_%s_public_id_%s", table, map[string]string{"INSERT": "insert", "UPDATE OF public_id": "update"}[action])
			statement := fmt.Sprintf("CREATE TRIGGER IF NOT EXISTS %s BEFORE %s ON %s WHEN NEW.public_id IS NULL OR NEW.public_id = '' BEGIN SELECT RAISE(ABORT, '%s.public_id is required'); END", name, action, table, table)
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("enforce %s.public_id with SQLite trigger: %w", table, err)
			}
		}
	} else if err := db.Migrator().AlterColumn(constrained, "PublicID"); err != nil {
		return fmt.Errorf("enforce %s.public_id constraint: %w", table, err)
	}
	return nil
}
