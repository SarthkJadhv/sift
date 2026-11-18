package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenSQLiteCreatesDatabase(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sift.db")

	db, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite() failed: %v", err)
	}
	defer db.Close()

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("database file was not created: %v", err)
	}

	if info.IsDir() {
		t.Fatal("expected database path to be a file")
	}

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("database ping failed: %v", err)
	}
}

func TestSQLiteWALMode(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sift.db")

	db, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite() failed: %v", err)
	}
	defer db.Close()

	var mode string

	err = db.DB.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		t.Fatalf("failed to read journal mode: %v", err)
	}

	if mode != "wal" {
		t.Fatalf("expected WAL mode, got %s", mode)
	}
}

func TestSQLiteForeignKeysEnabled(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sift.db")

	db, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenSQLite() failed: %v", err)
	}
	defer db.Close()

	var enabled int

	err = db.DB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled)
	if err != nil {
		t.Fatalf("failed to check foreign keys: %v", err)
	}

	if enabled != 1 {
		t.Fatal("foreign key enforcement is disabled")
	}

	_, err = db.DB.ExecContext(ctx, `
		CREATE TABLE parents (
			id INTEGER PRIMARY KEY
		)
	`)
	if err != nil {
		t.Fatalf("failed to create parents table: %v", err)
	}

	_, err = db.DB.ExecContext(ctx, `
		CREATE TABLE children (
			parent_id INTEGER,
			FOREIGN KEY (parent_id) REFERENCES parents(id)
		)
	`)
	if err != nil {
		t.Fatalf("failed to create children table: %v", err)
	}

	_, err = db.DB.ExecContext(
		ctx,
		"INSERT INTO children (parent_id) VALUES (?)",
		999,
	)

	if err == nil {
		t.Fatal("expected foreign key constraint violation")
	}
}

func TestSQLitePersistence(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sift.db")

	db, err := OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("first OpenSQLite() failed: %v", err)
	}

	_, err = db.DB.ExecContext(ctx, `
		CREATE TABLE persistence_test (
			id INTEGER PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		db.Close()
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = db.DB.ExecContext(
		ctx,
		"INSERT INTO persistence_test (value) VALUES (?)",
		"SIFT",
	)
	if err != nil {
		db.Close()
		t.Fatalf("failed to insert record: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("failed to close database: %v", err)
	}

	// Reopen the same database.
	db, err = OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatalf("second OpenSQLite() failed: %v", err)
	}
	defer db.Close()

	var value string

	err = db.DB.QueryRowContext(
		ctx,
		"SELECT value FROM persistence_test WHERE id = 1",
	).Scan(&value)

	if err != nil {
		t.Fatalf("failed to retrieve persisted record: %v", err)
	}

	if value != "SIFT" {
		t.Fatalf("expected SIFT, got %s", value)
	}
}

func TestOpenSQLiteRejectsInvalidPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	blockingFile := filepath.Join(root, "not-a-directory")

	if err := os.WriteFile(blockingFile, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(blockingFile, "sift.db")

	_, err := OpenSQLite(ctx, dbPath)

	if err == nil {
		t.Fatal("expected error for invalid database path")
	}
}
