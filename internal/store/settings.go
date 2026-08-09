package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// The advisor's model API key, submitted through the admin UI. One row in
// app_settings holding provider and key together as encrypted JSON: the
// provider names which API the key belongs to, and a key without its
// provider is unusable, so they live or die as a pair.
const settingAdvisorKey = "advisor_key"

type AdvisorKey struct {
	Provider string `json:"provider"`
	APIKey   string `json:"apiKey"`
	// Model is the owner's choice of model, empty for the provider default
	// (which is also what every key stored before the field existed means).
	Model string `json:"model,omitempty"`
}

// encryptAdvisorKey / decryptAdvisorKey are shared by the app-wide and the
// per-user rows — same JSON shape, same box, different table.
func (s *Store) encryptAdvisorKey(k AdvisorKey) (string, error) {
	plain, err := json.Marshal(k)
	if err != nil {
		return "", err
	}
	enc, err := s.box.Encrypt(string(plain))
	if err != nil {
		return "", fmt.Errorf("encrypting advisor key: %w", err)
	}
	return enc, nil
}

func (s *Store) decryptAdvisorKey(enc string) (*AdvisorKey, error) {
	plain, err := s.box.Decrypt(enc)
	if err != nil {
		return nil, fmt.Errorf("decrypting advisor key: %w", err)
	}
	var k AdvisorKey
	if err := json.Unmarshal([]byte(plain), &k); err != nil {
		return nil, fmt.Errorf("decoding advisor key: %w", err)
	}
	return &k, nil
}

// AdvisorKey returns the stored shared key, or nil when none has been saved.
func (s *Store) AdvisorKey(ctx context.Context) (*AdvisorKey, error) {
	var enc string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, settingAdvisorKey).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.decryptAdvisorKey(enc)
}

func (s *Store) SetAdvisorKey(ctx context.Context, k AdvisorKey) error {
	enc, err := s.encryptAdvisorKey(k)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		settingAdvisorKey, enc)
	return err
}

func (s *Store) DeleteAdvisorKey(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_settings WHERE key = ?`, settingAdvisorKey)
	return err
}

// How many tool round-trips one advisor question may take. Stored plain —
// it's a tuning knob, not a secret — in the same app_settings table.
const settingAdvisorMaxRounds = "advisor_max_rounds"

// AdvisorMaxRounds returns the stored cap, or 0 when unset (the API layer
// applies its default).
func (s *Store) AdvisorMaxRounds(ctx context.Context) (int, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, settingAdvisorMaxRounds).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("decoding advisor max rounds: %w", err)
	}
	return n, nil
}

func (s *Store) SetAdvisorMaxRounds(ctx context.Context, n int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		settingAdvisorMaxRounds, strconv.Itoa(n))
	return err
}

// UserAdvisorKey returns one user's personal key, or nil when they haven't
// saved one. Personal keys shadow the shared key for that user's requests
// only — they are never read for anyone else.
func (s *Store) UserAdvisorKey(ctx context.Context, userID int64) (*AdvisorKey, error) {
	var enc string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM user_advisor_keys WHERE user_id = ?`, userID).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.decryptAdvisorKey(enc)
}

func (s *Store) SetUserAdvisorKey(ctx context.Context, userID int64, k AdvisorKey) error {
	enc, err := s.encryptAdvisorKey(k)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO user_advisor_keys (user_id, value, updated_at) VALUES (?, ?, datetime('now'))
		ON CONFLICT(user_id) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		userID, enc)
	return err
}

func (s *Store) DeleteUserAdvisorKey(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_advisor_keys WHERE user_id = ?`, userID)
	return err
}
