package users

import (
	"os"
	"testing"

	"github.com/leihog/discord-bot/internal/database"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := "test_users.db"
	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("New db: %v", err)
	}
	if err := db.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.Remove(dbPath)
		os.Remove(dbPath + "-wal")
		os.Remove(dbPath + "-shm")
	})
	return New(db)
}

func TestEnsureUser_NewUser(t *testing.T) {
	s := setupTestStore(t)

	if err := s.EnsureUser("u1", "Alice"); err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}

	u, err := s.GetUser("u1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u == nil {
		t.Fatal("expected user, got nil")
	}
	if u.DisplayName != "Alice" {
		t.Errorf("display_name = %q, want %q", u.DisplayName, "Alice")
	}

	// New users should get the "user" role automatically.
	ok, err := s.HasRole("u1", "user")
	if err != nil {
		t.Fatalf("HasRole: %v", err)
	}
	if !ok {
		t.Error("expected new user to have 'user' role")
	}
}

func TestEnsureUser_Idempotent(t *testing.T) {
	s := setupTestStore(t)

	if err := s.EnsureUser("u1", "Alice"); err != nil {
		t.Fatalf("first EnsureUser: %v", err)
	}
	if err := s.EnsureUser("u1", "Alice Updated"); err != nil {
		t.Fatalf("second EnsureUser: %v", err)
	}

	// Display name should be updated.
	u, _ := s.GetUser("u1")
	if u.DisplayName != "Alice Updated" {
		t.Errorf("display_name = %q, want %q", u.DisplayName, "Alice Updated")
	}

	// Should still have exactly one "user" role entry.
	ok, _ := s.HasRole("u1", "user")
	if !ok {
		t.Error("expected 'user' role to persist after second EnsureUser")
	}
}

func TestHasRole_AddRole_RemoveRole(t *testing.T) {
	s := setupTestStore(t)
	_ = s.EnsureUser("u2", "Bob")

	ok, _ := s.HasRole("u2", "admin")
	if ok {
		t.Error("should not have admin role yet")
	}

	if err := s.AddRole("u2", "admin"); err != nil {
		t.Fatalf("AddRole: %v", err)
	}
	ok, _ = s.HasRole("u2", "admin")
	if !ok {
		t.Error("expected admin role after AddRole")
	}

	if err := s.RemoveRole("u2", "admin"); err != nil {
		t.Fatalf("RemoveRole: %v", err)
	}
	ok, _ = s.HasRole("u2", "admin")
	if ok {
		t.Error("expected admin role removed")
	}
}

func TestMeta(t *testing.T) {
	s := setupTestStore(t)
	_ = s.EnsureUser("u3", "Carol")

	_, found, _ := s.GetMeta("u3", "xp")
	if found {
		t.Error("should not have xp meta yet")
	}

	if err := s.SetMeta("u3", "xp", "100"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	val, found, err := s.GetMeta("u3", "xp")
	if err != nil {
		t.Fatalf("GetMeta: %v", err)
	}
	if !found {
		t.Fatal("expected xp meta to exist")
	}
	if val != "100" {
		t.Errorf("xp = %q, want %q", val, "100")
	}

	// Overwrite
	_ = s.SetMeta("u3", "xp", "200")
	val, _, _ = s.GetMeta("u3", "xp")
	if val != "200" {
		t.Errorf("xp after update = %q, want %q", val, "200")
	}

	all, err := s.GetAllMeta("u3")
	if err != nil {
		t.Fatalf("GetAllMeta: %v", err)
	}
	if all["xp"] != "200" {
		t.Errorf("GetAllMeta xp = %q, want %q", all["xp"], "200")
	}
}

func TestClaimAdmin_ValidToken(t *testing.T) {
	s := setupTestStore(t)

	if err := s.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// Fetch the generated token directly from bot_config.
	var token string
	if err := s.db.QueryRow(
		`SELECT value FROM bot_config WHERE key = ?`, claimTokenKey,
	).Scan(&token); err != nil {
		t.Fatalf("reading token: %v", err)
	}

	ok, err := s.ClaimAdmin("u4", "Dave", token)
	if err != nil {
		t.Fatalf("ClaimAdmin: %v", err)
	}
	if !ok {
		t.Fatal("expected ClaimAdmin to succeed")
	}

	hasAdmin, _ := s.HasRole("u4", "admin")
	if !hasAdmin {
		t.Error("expected u4 to have admin role after claiming")
	}
	hasOwner, _ := s.HasRole("u4", "owner")
	if !hasOwner {
		t.Error("expected u4 to have owner role after claiming")
	}

	// GetOwner should return the claimer.
	owner, err := s.GetOwner()
	if err != nil {
		t.Fatalf("GetOwner: %v", err)
	}
	if owner == nil || owner.ID != "u4" {
		t.Errorf("expected owner to be u4, got %v", owner)
	}

	// Token should be consumed.
	ok2, _ := s.ClaimAdmin("u5", "Eve", token)
	if ok2 {
		t.Error("expected second claim with same token to fail")
	}
}

func TestOwnerRole_CannotBeRemoved(t *testing.T) {
	s := setupTestStore(t)
	_ = s.EnsureUser("u8", "Heidi")
	_ = s.AddRole("u8", "owner")

	_ = s.RemoveRole("u8", "owner")

	hasOwner, _ := s.HasRole("u8", "owner")
	if !hasOwner {
		t.Error("owner role should be protected from removal")
	}

	// Non-owner roles can still be removed.
	_ = s.AddRole("u8", "admin")
	_ = s.RemoveRole("u8", "admin")
	hasAdmin, _ := s.HasRole("u8", "admin")
	if hasAdmin {
		t.Error("admin role should be removable")
	}
}

func TestClaimAdmin_InvalidToken(t *testing.T) {
	s := setupTestStore(t)
	_ = s.Bootstrap()

	ok, err := s.ClaimAdmin("u6", "Frank", "WRONGTOK")
	if err != nil {
		t.Fatalf("ClaimAdmin: %v", err)
	}
	if ok {
		t.Error("expected ClaimAdmin with wrong token to fail")
	}
}

func TestBootstrap_NoTokenWhenAdminExists(t *testing.T) {
	s := setupTestStore(t)
	_ = s.EnsureUser("u7", "Grace")
	_ = s.AddRole("u7", "owner")

	if err := s.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// No token should have been generated.
	var token string
	err := s.db.QueryRow(
		`SELECT value FROM bot_config WHERE key = ?`, claimTokenKey,
	).Scan(&token)
	if err == nil {
		t.Error("expected no claim token when admin already exists")
	}
}
