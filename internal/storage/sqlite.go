package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteDB struct {
	DB   *sql.DB
	Path string
}

// OpenSQLite opens a local SQLite database and applies
// the required connection settings.
func OpenSQLite(ctx context.Context, path string) (*SQLiteDB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path cannot be empty")
	}

	// Resolve the absolute database path.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}

	// Create the parent directory if it doesn't exist.
	if err := os.MkdirAll(filepath.Dir(absPath), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// Open the SQLite database.
	db, err := sql.Open("sqlite", absPath)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}

	// Configure the connection pool.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	// Verify database connectivity.
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping SQLite database: %w", err)
	}

	// Enable foreign-key enforcement.
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Configure database lock waiting.
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure busy timeout: %w", err)
	}

	// Enable Write-Ahead Logging.
	var journalMode string

	err = db.QueryRowContext(
		ctx,
		"PRAGMA journal_mode = WAL",
	).Scan(&journalMode)

	if err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	if !strings.EqualFold(journalMode, "wal") {
		db.Close()
		return nil, fmt.Errorf(
			"expected WAL journal mode, got %s",
			journalMode,
		)
	}

	return &SQLiteDB{
		DB:   db,
		Path: absPath,
	}, nil
}

// Close releases the database connection pool.
func (s *SQLiteDB) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}

	return s.DB.Close()
}

// Ping verifies database connectivity.
func (s *SQLiteDB) Ping(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return errors.New("SQLite database is not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return s.DB.PingContext(ctx)
}
