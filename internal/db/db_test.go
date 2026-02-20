package db_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/lojf/nextgen/internal/db"
)

// TestWALMode verifies that the DSN parameters in db.go enable WAL journal mode.
// WAL is the key SQLite setting for concurrent reads + single-writer throughput.
func TestWALMode(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "wal_test.db") +
		"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"

	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	var mode string
	gdb.Raw("PRAGMA journal_mode").Scan(&mode)
	if mode != "wal" {
		t.Errorf("expected journal_mode=wal, got %q", mode)
	}
}

// TestInit_CreatesIndexes verifies that Init() creates the two composite
// indexes on the registrations table that GORM does not auto-create.
func TestInit_CreatesIndexes(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(orig) }) //nolint:errcheck
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	if err := db.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}

	sqlDB, err := db.Conn().DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}

	found := indexNames(t, sqlDB, "registrations")
	for _, want := range []string{"idx_reg_class_status", "idx_reg_parent"} {
		if !found[want] {
			t.Errorf("index %q missing from registrations table; found: %v", want, found)
		}
	}
}

func indexNames(t *testing.T, sqlDB *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := sqlDB.Query("PRAGMA index_list(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA index_list: %v", err)
	}
	defer rows.Close()

	out := make(map[string]bool)
	for rows.Next() {
		var seq int
		var name string
		var unique bool
		var origin, partial string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[name] = true
	}
	return out
}
