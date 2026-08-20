package testdb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitDSN(t *testing.T) {
	base, query, err := splitDSN("root:secret@tcp(127.0.0.1:3306)/mysql?parseTime=true")
	if err != nil || base != "root:secret@tcp(127.0.0.1:3306)/" || query != "?parseTime=true" {
		t.Fatalf("splitDSN = %q, %q, %v", base, query, err)
	}
}

func TestMySQLTargetIsConcurrencyIsolated(t *testing.T) {
	makefile, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	compose, err := os.ReadFile(filepath.Join("..", "..", "docker-compose.test.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	wantMake := []string{"mktemp -d", "docker compose", " port mysql 3306", "test -json -race -count=1"}
	for _, want := range wantMake {
		if !strings.Contains(string(makefile), want) {
			t.Errorf("API Makefile must contain %q", want)
		}
	}
	if strings.Contains(string(makefile), "kumquat-api-test-$${USER") {
		t.Error("API MySQL target still reuses one Compose project per user")
	}
	if !strings.Contains(string(compose), `127.0.0.1::3306`) {
		t.Error("MySQL Compose service must publish an ephemeral host port")
	}
}
