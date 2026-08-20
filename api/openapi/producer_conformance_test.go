package openapi_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/fize/kumquat/api/pkg/dto"
	"github.com/fize/kumquat/api/pkg/model"
	"gopkg.in/yaml.v3"
)

func TestBusinessProducerKeysConformToConcreteSchemas(t *testing.T) {
	b, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Required   []string               `yaml:"required"`
				Properties map[string]interface{} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	role := model.Role{Base: model.Base{ID: 1, CreatedAt: now, UpdatedAt: now}, Name: model.RoleMember}
	module := model.Module{Base: model.Base{ID: 2, CreatedAt: now, UpdatedAt: now}, PublicID: "module_abcdef", Name: "team", Level: 1, Path: "/team"}
	project := model.Project{Base: model.Base{ID: 3, CreatedAt: now, UpdatedAt: now}, PublicID: "project_abcdef", Name: "project", ModuleID: module.ID, Module: module}
	user := model.User{Base: model.Base{ID: 4, CreatedAt: now, UpdatedAt: now}, Username: "member", Email: "member@example.test", Nickname: "Member", RoleID: role.ID, Role: role, ModuleID: &module.ID, Module: &module}
	cases := []struct {
		name, schema string
		value        map[string]interface{}
	}{
		{"role", "RoleDTO", role.ToResponse()}, {"module", "ModuleDTO", module.ToResponse()}, {"project", "ProjectDTO", project.ToResponse()}, {"user", "UserDTO", user.ToResponse()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema := doc.Components.Schemas[tc.schema]
			for key := range tc.value {
				if _, ok := schema.Properties[key]; !ok {
					t.Errorf("producer key %q absent from %s", key, tc.schema)
				}
			}
			for _, key := range schema.Required {
				if _, ok := tc.value[key]; !ok {
					t.Errorf("required schema key %q absent from producer", key)
				}
			}
		})
	}
}

func TestModuleAndPermissionTypedProducersFullyConform(t *testing.T) {
	b, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				AdditionalProperties interface{}            `yaml:"additionalProperties"`
				Required             []string               `yaml:"required"`
				Properties           map[string]interface{} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	parent := model.Module{Base: model.Base{ID: 1, CreatedAt: now, UpdatedAt: now}, PublicID: "module_parent", Name: "parent", Level: 1, Path: "/parent"}
	child := model.Module{Base: model.Base{ID: 2, CreatedAt: now, UpdatedAt: now}, PublicID: "module_child", Name: "child", ParentID: &parent.ID, Parent: &parent, Level: 2, Path: "/parent/child"}
	values := []struct {
		schema string
		value  interface{}
	}{{"ModuleDTO", dto.ModuleFromModel(child)}, {"PermissionDTO", dto.PermissionFromModel(model.Permission{Resource: model.ResourceProject, Action: model.ActionRead, Effect: model.EffectAllow})}}
	for _, tc := range values {
		raw, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatal(err)
		}
		var producer map[string]interface{}
		if err := json.Unmarshal(raw, &producer); err != nil {
			t.Fatal(err)
		}
		schema := doc.Components.Schemas[tc.schema]
		if value, ok := schema.AdditionalProperties.(bool); !ok || value {
			t.Fatalf("%s must forbid additional properties", tc.schema)
		}
		for key := range producer {
			if _, ok := schema.Properties[key]; !ok {
				t.Errorf("%s producer key %q is not contracted", tc.schema, key)
			}
		}
		for _, key := range schema.Required {
			if _, ok := producer[key]; !ok {
				t.Errorf("%s missing required key %q", tc.schema, key)
			}
		}
	}
}
