package database

import (
	"database/sql"
	"log"

	_ "modernc.org/sqlite"
)

// DB wraps the database connection
type DB struct {
	*sql.DB
}

// New creates a new database connection
func New(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, err
	}

	return &DB{db}, nil
}

// Initialize sets up the database schema
func (db *DB) Initialize() error {
	log.Println("Initializing database")

	_, err := db.Exec(`PRAGMA journal_mode=WAL;`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS kv_store (
		namespace TEXT NOT NULL,
		key TEXT NOT NULL,
		value TEXT,
		PRIMARY KEY (namespace, key)
	)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		display_name TEXT NOT NULL,
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS user_roles (
		user_id TEXT NOT NULL REFERENCES users(id),
		role TEXT NOT NULL,
		PRIMARY KEY (user_id, role)
	)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS user_meta (
		user_id TEXT NOT NULL REFERENCES users(id),
		key TEXT NOT NULL,
		value TEXT,
		PRIMARY KEY (user_id, key)
	)`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS bot_config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`)
	if err != nil {
		return err
	}

	log.Println("Database initialized successfully")
	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}
