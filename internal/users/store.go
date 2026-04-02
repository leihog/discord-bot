package users

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"log"
	"time"

	"github.com/leihog/discord-bot/internal/database"
)

const claimTokenKey = "admin_claim_token"
const tokenChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const tokenLen = 8

// User represents a bot user.
type User struct {
	ID          string
	DisplayName string
	Roles       []string
	CreatedAt   int64
}

// Store manages user persistence.
type Store struct {
	db *sql.DB
}

// New creates a Store backed by the given database.
func New(db *database.DB) *Store {
	return &Store{db: db.DB}
}

// EnsureUser inserts or updates a user record. New users receive the "user" role.
func (s *Store) EnsureUser(id, displayName string) error {
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO users(id, display_name, created_at) VALUES(?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET display_name=excluded.display_name`,
		id, displayName, now,
	)
	if err != nil {
		return err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	// RowsAffected == 1 means a fresh INSERT (new user); give them the "user" role.
	// On an UPDATE SQLite also reports 1 if the row changed, but the role insert
	// below is idempotent (INSERT OR IGNORE), so it's safe either way.
	if rows == 1 {
		if err := s.AddRole(id, "user"); err != nil {
			return err
		}
	}
	return nil
}

// GetUser returns the user with the given Discord ID, or nil if not found.
func (s *Store) GetUser(id string) (*User, error) {
	row := s.db.QueryRow(`SELECT id, display_name, created_at FROM users WHERE id = ?`, id)
	u := &User{}
	if err := row.Scan(&u.ID, &u.DisplayName, &u.CreatedAt); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`SELECT role FROM user_roles WHERE user_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		u.Roles = append(u.Roles, role)
	}
	return u, rows.Err()
}

// HasRole reports whether the user has the given role.
func (s *Store) HasRole(userID, role string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM user_roles WHERE user_id = ? AND role = ?`, userID, role,
	).Scan(&count)
	return count > 0, err
}

// AddRole grants a role to a user (idempotent).
func (s *Store) AddRole(userID, role string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO user_roles(user_id, role) VALUES(?, ?)`, userID, role,
	)
	return err
}

// RemoveRole revokes a role from a user. The "owner" role is protected and
// cannot be removed.
func (s *Store) RemoveRole(userID, role string) error {
	if role == "owner" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM user_roles WHERE user_id = ? AND role = ?`, userID, role)
	return err
}

// GetOwner returns the user with the "owner" role, or nil if unclaimed.
func (s *Store) GetOwner() (*User, error) {
	var ownerID string
	err := s.db.QueryRow(`SELECT user_id FROM user_roles WHERE role = 'owner'`).Scan(&ownerID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.GetUser(ownerID)
}

// SetMeta stores a metadata value for a user.
func (s *Store) SetMeta(userID, key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO user_meta(user_id, key, value) VALUES(?, ?, ?)
		 ON CONFLICT(user_id, key) DO UPDATE SET value=excluded.value`,
		userID, key, value,
	)
	return err
}

// GetMeta retrieves a metadata value. Returns ("", false, nil) if not found.
func (s *Store) GetMeta(userID, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(
		`SELECT value FROM user_meta WHERE user_id = ? AND key = ?`, userID, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return value, err == nil, err
}

// GetAllMeta returns all metadata entries for a user.
func (s *Store) GetAllMeta(userID string) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM user_meta WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	meta := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		meta[k] = v
	}
	return meta, rows.Err()
}

// Bootstrap checks for an existing owner. If none exists it generates (or re-logs)
// a one-time claim token that the owner can use via !claim_admin.
func (s *Store) Bootstrap() error {
	var ownerCount int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM user_roles WHERE role = 'owner'`,
	).Scan(&ownerCount); err != nil {
		return err
	}
	if ownerCount > 0 {
		return nil
	}

	// Check for an existing token (bot restarted before anyone claimed admin).
	var existing string
	err := s.db.QueryRow(
		`SELECT value FROM bot_config WHERE key = ?`, claimTokenKey,
	).Scan(&existing)

	if err == sql.ErrNoRows {
		token, err := generateToken()
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(
			`INSERT INTO bot_config(key, value) VALUES(?, ?)`, claimTokenKey, token,
		); err != nil {
			return err
		}
		existing = token
	} else if err != nil {
		return err
	}

	log.Printf("[ADMIN BOOTSTRAP] No admin set. DM the bot: !claim_admin %s", existing)
	return nil
}

// ClaimAdmin validates the token and grants the caller admin rights.
// Returns true on success, false if the token is missing or wrong.
func (s *Store) ClaimAdmin(userID, displayName, token string) (bool, error) {
	var stored string
	err := s.db.QueryRow(
		`SELECT value FROM bot_config WHERE key = ?`, claimTokenKey,
	).Scan(&stored)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// Constant-time comparison to avoid timing attacks.
	if subtle.ConstantTimeCompare([]byte(token), []byte(stored)) != 1 {
		return false, nil
	}

	if err := s.EnsureUser(userID, displayName); err != nil {
		return false, err
	}
	if err := s.AddRole(userID, "owner"); err != nil {
		return false, err
	}
	if err := s.AddRole(userID, "admin"); err != nil {
		return false, err
	}
	_, err = s.db.Exec(`DELETE FROM bot_config WHERE key = ?`, claimTokenKey)
	return err == nil, err
}

func generateToken() (string, error) {
	b := make([]byte, tokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, tokenLen)
	for i, v := range b {
		out[i] = tokenChars[int(v)%len(tokenChars)]
	}
	return string(out), nil
}
