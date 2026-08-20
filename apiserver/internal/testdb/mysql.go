package testdb

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const EnvDSN = "KUMQUAT_TEST_MYSQL_DSN"

var databaseSequence atomic.Uint64
var invalidDatabaseCharacter = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// Open creates a dedicated real-MySQL database for one test and drops only
// that database when the test completes. The authoritative MySQL test target
// sets EnvDSN; ordinary unit-only runs skip these integration tests explicitly.
func Open(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(EnvDSN)
	if dsn == "" {
		t.Skipf("real MySQL integration test requires %s; run make test-api-mysql", EnvDSN)
	}

	base, query, err := splitDSN(dsn)
	if err != nil {
		t.Fatalf("invalid %s: %v", EnvDSN, err)
	}
	name := invalidDatabaseCharacter.ReplaceAllString(strings.ToLower(t.Name()), "_")
	name = strings.Trim(name, "_")
	if len(name) > 32 {
		name = name[:32]
	}
	name = fmt.Sprintf("kumquat_%s_%d_%d", name, time.Now().UnixNano(), databaseSequence.Add(1))
	if len(name) > 63 {
		name = name[:63]
	}

	admin, err := gorm.Open(mysql.Open(base+"mysql"+query), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect to MySQL test server: %v", err)
	}
	if err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		t.Fatalf("create isolated MySQL database %s: %v", name, err)
	}
	t.Cleanup(func() {
		if err := admin.Exec("DROP DATABASE IF EXISTS `" + name + "`").Error; err != nil {
			t.Errorf("drop isolated MySQL database %s: %v", name, err)
		}
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	db, err := gorm.Open(mysql.Open(base+name+query), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open isolated MySQL database %s: %v", name, err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func splitDSN(dsn string) (base, query string, err error) {
	queryIndex := strings.IndexByte(dsn, '?')
	withoutQuery := dsn
	if queryIndex >= 0 {
		withoutQuery, query = dsn[:queryIndex], dsn[queryIndex:]
	}
	slash := strings.LastIndexByte(withoutQuery, '/')
	if slash < 0 || !strings.Contains(withoutQuery[:slash], "@tcp(") {
		return "", "", fmt.Errorf("expected user:password@tcp(host:port)/database DSN")
	}
	return withoutQuery[:slash+1], query, nil
}
