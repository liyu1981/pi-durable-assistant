package settings

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Profile is one named, stored LLM provider configuration.
type Profile struct {
	ID     int64
	Name   string
	Config Config
	Active bool
}

const profileSelect = `SELECT id, name, api, base_url, api_key, model, context_window, max_tokens, updated_at, is_active FROM app_llm_profiles`

func scanProfile(row interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	var active int
	err := row.Scan(&p.ID, &p.Name, &p.Config.API, &p.Config.BaseURL, &p.Config.APIKey,
		&p.Config.Model, &p.Config.ContextWindow, &p.Config.MaxTokens, &p.Config.UpdatedAt, &active)
	p.Active = active == 1
	return p, err
}

// ensureProfiles copies the legacy single config into a profile on upgrade.
func (s *Store) ensureProfiles() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM app_llm_profiles`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	cfg, err := s.LoadLegacy()
	if err != nil {
		return err
	}
	name := "Default"
	if cfg.Model != "" {
		name = cfg.Model
	}
	stamp := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`INSERT INTO app_llm_profiles
		(name, api, base_url, api_key, model, context_window, max_tokens, updated_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		name, cfg.API, cfg.BaseURL, cfg.APIKey, cfg.Model,
		cfg.ContextWindow, cfg.MaxTokens, stamp)
	return err
}

// LoadLegacy reads the single compatibility row.
func (s *Store) LoadLegacy() (Config, error) {
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

// ListProfiles returns all profiles, active first.
func (s *Store) ListProfiles() ([]Profile, error) {
	rows, err := s.db.Query(profileSelect + ` ORDER BY is_active DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := []Profile{}
	for rows.Next() {
		profile, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

// GetProfile returns one profile by id.
func (s *Store) GetProfile(id int64) (Profile, error) {
	return scanProfile(s.db.QueryRow(profileSelect+` WHERE id = ?`, id))
}

// CreateProfile stores a new inactive profile.
func (s *Store) CreateProfile(name string, cfg Config) (Profile, error) {
	if err := cfg.Validate(); err != nil {
		return Profile{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, fmt.Errorf("profile name is required")
	}
	if len(name) > 80 {
		return Profile{}, fmt.Errorf("profile name is too long")
	}
	cfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec(`INSERT INTO app_llm_profiles
		(name, api, base_url, api_key, model, context_window, max_tokens, updated_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		name, cfg.API, cfg.BaseURL, cfg.APIKey, cfg.Model,
		cfg.ContextWindow, cfg.MaxTokens, cfg.UpdatedAt)
	if err != nil {
		return Profile{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Profile{}, err
	}
	return s.GetProfile(id)
}

// UpdateProfile updates a profile's name and configuration. An empty name
// keeps the current one; an empty API key keeps the stored key.
func (s *Store) UpdateProfile(id int64, name *string, cfg Config) (Profile, error) {
	if err := cfg.Validate(); err != nil {
		return Profile{}, err
	}
	existing, err := s.GetProfile(id)
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, fmt.Errorf("profile %d not found", id)
	}
	if err != nil {
		return Profile{}, err
	}
	finalName := existing.Name
	if name != nil {
		finalName = strings.TrimSpace(*name)
		if finalName == "" || len(finalName) > 80 {
			return Profile{}, fmt.Errorf("invalid profile name")
		}
	}
	cfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.Exec(`UPDATE app_llm_profiles SET
		name = ?, api = ?, base_url = ?, api_key = ?, model = ?, context_window = ?, max_tokens = ?, updated_at = ?
		WHERE id = ?`,
		finalName, cfg.API, cfg.BaseURL, cfg.APIKey, cfg.Model,
		cfg.ContextWindow, cfg.MaxTokens, cfg.UpdatedAt, id)
	if err != nil {
		return Profile{}, err
	}
	if existing.Active {
		if err := s.Save(cfg); err != nil {
			return Profile{}, err
		}
	}
	return s.GetProfile(id)
}

// DeleteProfile removes a profile. The last profile cannot be deleted.
func (s *Store) DeleteProfile(id int64) error {
	profiles, err := s.ListProfiles()
	if err != nil {
		return err
	}
	if len(profiles) <= 1 {
		return fmt.Errorf("cannot delete the last profile")
	}
	var target *Profile
	for i := range profiles {
		if profiles[i].ID == id {
			target = &profiles[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("profile %d not found", id)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM app_llm_profiles WHERE id = ?`, id); err != nil {
		_ = tx.Rollback()
		return err
	}
	if target.Active {
		for _, candidate := range profiles {
			if candidate.ID != id {
				if _, err := tx.Exec(`UPDATE app_llm_profiles SET is_active = 1 WHERE id = ?`, candidate.ID); err != nil {
					_ = tx.Rollback()
					return err
				}
				break
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if target.Active {
		cfg, err := s.Load()
		if err != nil {
			return err
		}
		return s.Save(cfg)
	}
	return nil
}

// ActivateProfile makes one profile active and syncs the compatibility row.
func (s *Store) ActivateProfile(id int64) (Config, error) {
	profile, err := s.GetProfile(id)
	if errors.Is(err, sql.ErrNoRows) {
		return Config{}, fmt.Errorf("profile %d not found", id)
	}
	if err != nil {
		return Config{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Config{}, err
	}
	if _, err := tx.Exec(`UPDATE app_llm_profiles SET is_active = 0`); err != nil {
		_ = tx.Rollback()
		return Config{}, err
	}
	if _, err := tx.Exec(`UPDATE app_llm_profiles SET is_active = 1 WHERE id = ?`, id); err != nil {
		_ = tx.Rollback()
		return Config{}, err
	}
	if err := tx.Commit(); err != nil {
		return Config{}, err
	}
	if err := s.Save(profile.Config); err != nil {
		return Config{}, err
	}
	return profile.Config, nil
}
