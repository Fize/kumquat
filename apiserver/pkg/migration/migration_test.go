package migration

import (
	"testing"

	"github.com/fize/kumquat/apiserver/internal/testdb"
	"github.com/fize/kumquat/apiserver/pkg/model"
)

func TestMigrateBackfillsLegacyPublicIDsWithoutLosingReferences(t *testing.T) {
	db := testdb.Open(t)
	statements := []string{
		`CREATE TABLE modules (id bigint unsigned PRIMARY KEY AUTO_INCREMENT, created_at datetime(3), updated_at datetime(3), deleted_at datetime(3), name varchar(191) NOT NULL, parent_id bigint unsigned, level integer NOT NULL DEFAULT 1, sort integer DEFAULT 0, path text)`,
		`CREATE TABLE projects (id bigint unsigned PRIMARY KEY AUTO_INCREMENT, created_at datetime(3), updated_at datetime(3), deleted_at datetime(3), name varchar(191) NOT NULL, module_id bigint unsigned NOT NULL, config longtext)`,
		`INSERT INTO modules (id, name, level, path) VALUES (7, 'legacy-module', 1, '/legacy-module')`,
		`INSERT INTO projects (id, name, module_id) VALUES (11, 'legacy-project', 7)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("migration is not idempotent: %v", err)
	}
	var module model.Module
	if err := db.First(&module, 7).Error; err != nil {
		t.Fatal(err)
	}
	var project model.Project
	if err := db.Preload("Module").First(&project, 11).Error; err != nil {
		t.Fatal(err)
	}
	if module.PublicID == "" || project.PublicID == "" || project.ModuleID != module.ID || project.Module.PublicID != module.PublicID {
		t.Fatalf("migration lost identity/reference: module=%#v project=%#v", module, project)
	}
	if err := db.Exec("UPDATE modules SET public_id = NULL WHERE id = ?", module.ID).Error; err == nil {
		t.Fatal("modules.public_id still accepts NULL")
	}
	duplicate := model.Module{Name: "duplicate", PublicID: module.PublicID}
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("modules.public_id is not unique")
	}
	if err := db.Exec("UPDATE projects SET public_id = NULL WHERE id = ?", project.ID).Error; err == nil {
		t.Fatal("projects.public_id still accepts NULL")
	}
	duplicateProject := model.Project{Name: "duplicate", ModuleID: module.ID, PublicID: project.PublicID}
	if err := db.Omit("config").Create(&duplicateProject).Error; err == nil {
		t.Fatal("projects.public_id is not unique")
	}
}
