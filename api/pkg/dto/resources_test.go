package dto

import (
	"encoding/json"
	"testing"
)

func TestResourceSecretRefJSONContract(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"secretRef":{"name":"runtime-env","key":"TOKEN"}}`), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["secretRef"] == nil {
		t.Fatal("secretRef API payload was not retained")
	}
}
