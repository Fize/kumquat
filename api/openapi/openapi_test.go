package openapi_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestContractIsOpenAPI3AndHasUniqueOperations(t *testing.T) {
	b, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		OpenAPI string                          `yaml:"openapi"`
		Paths   map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.OpenAPI) < 2 || doc.OpenAPI[0] != '3' {
		t.Fatalf("expected OpenAPI 3.x, got %q", doc.OpenAPI)
	}
	seen := map[string]string{}
	for path, methods := range doc.Paths {
		for method, node := range methods {
			if method == "parameters" {
				continue
			}
			var op struct {
				OperationID string `yaml:"operationId"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("decode %s %s: %v", method, path, err)
			}
			if op.OperationID == "" {
				t.Fatalf("%s %s has no operationId", method, path)
			}
			if previous, ok := seen[op.OperationID]; ok {
				t.Fatalf("duplicate operationId %q at %s and %s", op.OperationID, previous, path)
			}
			seen[op.OperationID] = path
		}
	}
	for _, id := range []string{"createApplication", "createWorkspace", "adoptCluster", "getOperation"} {
		if _, ok := seen[id]; !ok {
			t.Errorf("missing operation %s", id)
		}
	}
}

func TestEveryReferenceResolvesAndBusinessSuccessIsConcrete(t *testing.T) {
	b, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	var walk func(interface{})
	walk = func(value interface{}) {
		switch typed := value.(type) {
		case map[string]interface{}:
			for key, child := range typed {
				if key == "$ref" {
					ref, ok := child.(string)
					if !ok || !strings.HasPrefix(ref, "#/components/") {
						t.Fatalf("invalid ref %#v", child)
					}
					cursor := interface{}(root)
					for _, segment := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
						node, ok := cursor.(map[string]interface{})
						if !ok {
							t.Fatalf("ref %s traverses non-object", ref)
						}
						cursor, ok = node[segment]
						if !ok {
							t.Fatalf("unresolved ref %s", ref)
						}
					}
				}
				walk(child)
			}
		case []interface{}:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(root)
	paths := root["paths"].(map[string]interface{})
	for path, pathValue := range paths {
		for method, operation := range pathValue.(map[string]interface{}) {
			if method == "parameters" {
				continue
			}
			responses := operation.(map[string]interface{})["responses"].(map[string]interface{})
			for status, response := range responses {
				if !strings.HasPrefix(status, "2") {
					continue
				}
				ref := response.(map[string]interface{})["$ref"]
				if ref == "#/components/responses/Success" {
					t.Fatalf("%s %s uses generic success", method, path)
				}
			}
		}
	}
	if reflect.DeepEqual(root["components"].(map[string]interface{})["schemas"].(map[string]interface{})["EnvironmentVariable"], nil) {
		t.Fatal("missing environment schema")
	}
}

func TestClusterResourceUsesTypedEmptyDesiredState(t *testing.T) {
	b, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]yaml.Node `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var empty struct {
		Type                 string `yaml:"type"`
		AdditionalProperties bool   `yaml:"additionalProperties"`
		MaxProperties        int    `yaml:"maxProperties"`
	}
	emptyNode := doc.Components.Schemas["EmptyDesiredState"]
	if err := emptyNode.Decode(&empty); err != nil {
		t.Fatal(err)
	}
	if empty.Type != "object" || empty.AdditionalProperties || empty.MaxProperties != 0 {
		t.Fatalf("empty cluster desired schema = %#v", empty)
	}
	var cluster map[string]interface{}
	clusterNode := doc.Components.Schemas["ClusterResource"]
	if err := clusterNode.Decode(&cluster); err != nil {
		t.Fatal(err)
	}
	encoded, err := yaml.Marshal(cluster)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "#/components/schemas/EmptyDesiredState") {
		t.Fatalf("ClusterResource does not reference EmptyDesiredState: %s", encoded)
	}
}
