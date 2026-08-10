package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/safwyls/wildskeeper/internal/crypto"
	"github.com/safwyls/wildskeeper/internal/db"
)

func newTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	sqlDB, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	box, err := crypto.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("crypto: %v", err)
	}
	return New(sqlDB, box), sqlDB
}

func TestServerCRUDRoundTripWithEncryption(t *testing.T) {
	st, sqlDB := newTestStore(t)
	ctx := context.Background()

	id, err := st.CreateServer(ctx, &Server{
		Name: "main", Host: "10.0.0.5",
		RCONPort: 25575, RCONPassword: "rcon-secret",
		RESTPort: 8212, RESTPassword: "rest-secret",
		UseREST: true, Enabled: true,
		SavePath: "/saves", ConfigPath: "/config", ContainerName: "palworld",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := st.GetServer(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RCONPassword != "rcon-secret" || got.RESTPassword != "rest-secret" {
		t.Errorf("passwords did not round-trip: %q %q", got.RCONPassword, got.RESTPassword)
	}
	if got.Name != "main" || !got.UseREST || !got.Enabled || got.SavePath != "/saves" {
		t.Errorf("fields did not round-trip: %+v", got)
	}

	// Encrypted at rest: the raw columns must not contain the plaintext.
	var rconEnc, restEnc string
	err = sqlDB.QueryRow(`SELECT rcon_password_enc, rest_password_enc FROM servers WHERE id = ?`, id).
		Scan(&rconEnc, &restEnc)
	if err != nil {
		t.Fatalf("raw read: %v", err)
	}
	if strings.Contains(rconEnc, "rcon-secret") || strings.Contains(restEnc, "rest-secret") {
		t.Error("passwords stored in plaintext")
	}

	if err := st.DeleteServer(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetServer(ctx, id); err != ErrNotFound {
		t.Errorf("after delete: got %v, want ErrNotFound", err)
	}
}

// Blank passwords on update mean "keep the stored one" — the contract the
// edit dialog relies on so admins never re-enter credentials.
func TestUpdateServerPreservesBlankPasswords(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()

	id, err := st.CreateServer(ctx, &Server{
		Name: "main", Host: "h", RCONPassword: "old-rcon", RESTPassword: "old-rest",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := st.UpdateServer(ctx, &Server{ID: id, Name: "renamed", Host: "h2"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := st.GetServer(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "renamed" || got.Host != "h2" {
		t.Errorf("update did not apply: %+v", got)
	}
	if got.RCONPassword != "old-rcon" || got.RESTPassword != "old-rest" {
		t.Errorf("blank update clobbered passwords: %q %q", got.RCONPassword, got.RESTPassword)
	}

	// A non-blank password does replace.
	if err := st.UpdateServer(ctx, &Server{ID: id, Name: "renamed", Host: "h2", RCONPassword: "new-rcon"}); err != nil {
		t.Fatalf("update password: %v", err)
	}
	got, _ = st.GetServer(ctx, id)
	if got.RCONPassword != "new-rcon" || got.RESTPassword != "old-rest" {
		t.Errorf("password update wrong: %q %q", got.RCONPassword, got.RESTPassword)
	}
}

func TestPermissionEncodeDecode(t *testing.T) {
	// Unknown grants are dropped on encode; order and known grants survive.
	enc := encodePermissions([]string{PermPower, "not-a-permission", PermModerate})
	if enc != PermPower+","+PermModerate {
		t.Errorf("encode = %q", enc)
	}
	if got := decodePermissions(enc); !reflect.DeepEqual(got, []string{PermPower, PermModerate}) {
		t.Errorf("decode = %v", got)
	}
	if got := decodePermissions(""); got != nil {
		t.Errorf("decode empty = %v, want nil", got)
	}
	if got := decodePermissions(" , "); len(got) != 0 {
		t.Errorf("decode blanks = %v, want empty", got)
	}
}

func TestUserPermissionsRoundTrip(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()

	id, err := st.CreateUser(ctx, "carol", "hash", "user", []string{PermBroadcast, "bogus", PermSave})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	u, err := st.GetUser(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !reflect.DeepEqual(u.Permissions, []string{PermBroadcast, PermSave}) {
		t.Errorf("permissions = %v", u.Permissions)
	}
	if !u.Can(PermBroadcast) || u.Can(PermPower) {
		t.Errorf("Can() wrong: %v", u.Permissions)
	}

	// Disabled users can do nothing regardless of grants; admins everything.
	u.Disabled = true
	if u.Can(PermBroadcast) {
		t.Error("disabled user passed Can()")
	}
	admin := &User{Role: RoleAdmin}
	if !admin.Can(PermSettings) {
		t.Error("admin failed Can()")
	}
}
