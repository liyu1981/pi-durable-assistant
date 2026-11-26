package settings

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	// Registers the pure-Go "sqlite" database/sql driver, the same driver PiG
	// uses for agent.sqlite.
	_ "modernc.org/sqlite"
)

// migration is one ordered schema step. Versions are sequential and recorded
// in SQLite's user_version; never edit a shipped step, append a new one.
type migration struct {
	version   int
	name      string
	statement string
}

var migrations = []migration{
	{
		version: 1,
		name:    "llm_config",
		statement: `CREATE TABLE IF NOT EXISTS app_llm_config (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			api TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL,
			context_window INTEGER NOT NULL,
			max_tokens INTEGER NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	},
}

// Store owns the settings connection to the application's SQLite database.
type Store struct {
	db *sql.DB
}

// Open applies pending migrations and seeds the single settings row from
// defaults when the database is new. Reopening an existing database never
// overwrites what the user configured in the UI.
func Open(path string, defaults Config) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection keeps this pool's writes serialized; PiG has its own
	// connection to the same WAL database.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.seed(defaults); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this build supports (%d)", version, len(migrations))
	}
	for _, step := range migrations[version:] {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(step.statement); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", step.version, step.name, err)
		}
		// Setting user_version in the same transaction makes a failed step
		// retryable on the next start.
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, step.version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", step.version, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seed(defaults Config) error {
	stamp := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT OR IGNORE INTO app_llm_config
		(id, api, base_url, api_key, model, context_window, max_tokens, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?)`,
		defaults.API, defaults.BaseURL, defaults.APIKey, defaults.Model,
		defaults.ContextWindow, defaults.MaxTokens, stamp)
	return err
}

// Load returns the single stored configuration.
func (s *Store) Load() (Config, error) {
	var cfg Config
	err := s.db.QueryRow(`SELECT api, base_url, api_key, model, context_window, max_tokens, updated_at
		FROM app_llm_config WHERE id = 1`).Scan(
		&cfg.API, &cfg.BaseURL, &cfg.APIKey, &cfg.Model,
		&cfg.ContextWindow, &cfg.MaxTokens, &cfg.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Defaults(), nil
	}
	return cfg, err
}

// Save replaces the stored configuration. Validation happens before this in
// the API layer; the store also checks so direct callers cannot persist junk.
func (s *Store) Save(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	cfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`UPDATE app_llm_config SET
		api = ?, base_url = ?, api_key = ?, model = ?, context_window = ?, max_tokens = ?, updated_at = ?
		WHERE id = 1`,
		cfg.API, cfg.BaseURL, cfg.APIKey, cfg.Model,
		cfg.ContextWindow, cfg.MaxTokens, cfg.UpdatedAt)
	return err
}

// Close releases the database connection.
func (s *Store) Close() error { return s.db.Close() }
