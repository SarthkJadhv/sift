package storage

import (
	"context"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func openMigrationTestDB(t *testing.T) (*SQLiteDB, context.Context) {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite() failed: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})

	return db, ctx
}

func TestMigrateCreatesProjectsTable(t *testing.T) {
	db, ctx := openMigrationTestDB(t)

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	var count int

	err := db.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table'
		AND name = 'projects'
	`).Scan(&count)

	if err != nil {
		t.Fatalf("failed to inspect projects table: %v", err)
	}

	if count != 1 {
		t.Fatal("expected projects table to exist")
	}
}

func TestMigrationIsRecorded(t *testing.T) {
	db, ctx := openMigrationTestDB(t)

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate() failed: %v", err)
	}

	applied, err := db.MigrationApplied(ctx, 1)
	if err != nil {
		t.Fatalf("MigrationApplied() failed: %v", err)
	}

	if !applied {
		t.Fatal("expected migration version 1 to be recorded")
	}

	count, err := db.AppliedMigrationCount(ctx)
	if err != nil {
		t.Fatalf("AppliedMigrationCount() failed: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 applied migrations, got %d", count)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db, ctx := openMigrationTestDB(t)

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("first Migrate() failed: %v", err)
	}

	var firstAppliedAt string

	err := db.DB.QueryRowContext(
		ctx,
		"SELECT applied_at FROM schema_migrations WHERE version = 1",
	).Scan(&firstAppliedAt)

	if err != nil {
		t.Fatalf("failed to read first migration timestamp: %v", err)
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second Migrate() failed: %v", err)
	}

	count, err := db.AppliedMigrationCount(ctx)
	if err != nil {
		t.Fatalf("AppliedMigrationCount() failed: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 migrations after rerun, got %d", count)
	}

	var secondAppliedAt string

	err = db.DB.QueryRowContext(
		ctx,
		"SELECT applied_at FROM schema_migrations WHERE version = 1",
	).Scan(&secondAppliedAt)

	if err != nil {
		t.Fatalf("failed to read second migration timestamp: %v", err)
	}

	if firstAppliedAt != secondAppliedAt {
		t.Fatal("migration was unexpectedly reapplied")
	}
}

func TestApplyMigrationRollsBackOnTrackingFailure(t *testing.T) {
	db, ctx := openMigrationTestDB(t)

	_, err := db.DB.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create migration tracking table: %v", err)
	}

	// Force migration tracking to fail after migration SQL executes.
	_, err = db.DB.ExecContext(ctx, `
		CREATE TRIGGER reject_migration
		BEFORE INSERT ON schema_migrations
		BEGIN
			SELECT RAISE(ABORT, 'forced tracking failure');
		END;
	`)
	if err != nil {
		t.Fatalf("failed to create test trigger: %v", err)
	}

	m := migration{
		Version: 99,
		Name:    "099_test_rollback.sql",
		SQL:     "CREATE TABLE rollback_test (id INTEGER PRIMARY KEY)",
	}

	err = db.applyMigration(ctx, m)

	if err == nil {
		t.Fatal("expected migration tracking failure")
	}

	var count int

	err = db.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table'
		AND name = 'rollback_test'
	`).Scan(&count)

	if err != nil {
		t.Fatalf("failed to inspect rollback table: %v", err)
	}

	if count != 0 {
		t.Fatal("expected migration SQL to be rolled back")
	}
}

func TestLoadMigrationsSortsByVersion(t *testing.T) {
	source := fstest.MapFS{
		"migrations/010_add_tasks.sql": {
			Data: []byte("CREATE TABLE tasks (id INTEGER PRIMARY KEY);"),
		},
		"migrations/002_add_sessions.sql": {
			Data: []byte("CREATE TABLE sessions (id INTEGER PRIMARY KEY);"),
		},
		"migrations/001_create_projects.sql": {
			Data: []byte("CREATE TABLE projects (id INTEGER PRIMARY KEY);"),
		},
	}

	migrations, err := loadMigrationsFrom(source)
	if err != nil {
		t.Fatalf("loadMigrationsFrom() failed: %v", err)
	}

	if len(migrations) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(migrations))
	}

	expectedVersions := []int{1, 2, 10}

	for i, expected := range expectedVersions {
		if migrations[i].Version != expected {
			t.Errorf(
				"expected version %d at position %d, got %d",
				expected,
				i,
				migrations[i].Version,
			)
		}
	}
}

func TestLoadMigrationsRejectsInvalidFiles(t *testing.T) {
	tests := []struct {
		name  string
		files fstest.MapFS
	}{
		{
			name: "invalid filename",
			files: fstest.MapFS{
				"migrations/invalid.sql": {
					Data: []byte("CREATE TABLE test (id INTEGER);"),
				},
			},
		},
		{
			name: "invalid version",
			files: fstest.MapFS{
				"migrations/abc_test.sql": {
					Data: []byte("CREATE TABLE test (id INTEGER);"),
				},
			},
		},
		{
			name: "duplicate version",
			files: fstest.MapFS{
				"migrations/001_create_test.sql": {
					Data: []byte("CREATE TABLE test_a (id INTEGER);"),
				},
				"migrations/001_add_test.sql": {
					Data: []byte("CREATE TABLE test_b (id INTEGER);"),
				},
			},
		},
		{
			name: "empty migration",
			files: fstest.MapFS{
				"migrations/001_empty.sql": {
					Data: []byte("   "),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadMigrationsFrom(tt.files)

			if err == nil {
				t.Fatal("expected migration validation error")
			}
		})
	}
}
