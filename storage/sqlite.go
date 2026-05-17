package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattn/go-sqlite3"
)

// DB wraps the standard sql.DB with project-specific helpers.
type DB struct {
	*sql.DB
}

// Open loads the sqlite-vec extension and returns a ready DB handle.
//
// vecExtPath is the path to the vec0 shared library (vec0.dll on Windows,
// vec0.so on Linux). If empty, it defaults to looking for the library in a
// "bin/" directory relative to the current working directory.
func Open(dbPath string, vecExtPath string) (*DB, error) {
	if vecExtPath == "" {
		// Default: look for vec0 in bin/ next to the binary.
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("storage.Open: could not determine working directory: %w", err)
		}
		vecExtPath = filepath.Join(cwd, "bin", "vec0")
	}

	// Register a custom sqlite3 driver that loads the vec0 extension.
	// The driver name is unique so it can be registered once per process.
	driverName := "sqlite3_with_vec"
	// Guard against duplicate registration (e.g., in tests).
	for _, d := range sql.Drivers() {
		if d == driverName {
			goto openDB
		}
	}
	sql.Register(driverName, &sqlite3.SQLiteDriver{
		Extensions: []string{vecExtPath},
	})

openDB:
	db, err := sql.Open(driverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("storage.Open: failed to open db at %q: %w", dbPath, err)
	}

	// Verify the connection and that vec0 was loaded correctly.
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("storage.Open: ping failed (check vec0 extension path %q): %w", vecExtPath, err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, fmt.Errorf("storage.Open: failed to set WAL mode: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		return nil, fmt.Errorf("storage.Open: failed to enable foreign keys: %w", err)
	}

	return &DB{db}, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// ApplySchema runs the contents of schema.sql against the database.
// Typically used during seed DB setup and tests.
func (db *DB) ApplySchema(schemaSQL string) error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("storage.ApplySchema: %w", err)
	}
	return nil
}
