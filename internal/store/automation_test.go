package store

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func testServerID(t *testing.T, st *Store) int64 {
	t.Helper()
	id, err := st.CreateServer(context.Background(), &Server{
		Name: "main", Host: "10.0.0.5", RCONPort: 25575, RESTPort: 8212, UseREST: true, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return id
}

func TestRestartScheduleRoundTrip(t *testing.T) {
	st, _ := newTestStore(t)
	ctx := context.Background()
	serverID := testServerID(t, st)

	id, err := st.CreateRestartSchedule(ctx, &RestartSchedule{
		ServerID: serverID, Enabled: true,
		Days: []int{0, 3, 6}, TimeOfDay: "05:00", WarningMinutes: []int{15, 5, 1},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := st.GetRestartSchedule(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !reflect.DeepEqual(got.Days, []int{0, 3, 6}) || !reflect.DeepEqual(got.WarningMinutes, []int{15, 5, 1}) {
		t.Errorf("CSV fields did not round-trip: %+v", got)
	}
	if got.TimeOfDay != "05:00" || !got.Enabled || got.LastRunAt != nil {
		t.Errorf("fields did not round-trip: %+v", got)
	}

	at := time.Now().UTC().Truncate(time.Second)
	if err := st.MarkRestartScheduleRun(ctx, id, at); err != nil {
		t.Fatalf("mark run: %v", err)
	}
	got, err = st.GetRestartSchedule(ctx, id)
	if err != nil {
		t.Fatalf("get after run: %v", err)
	}
	if got.LastRunAt == nil || !got.LastRunAt.Equal(at) {
		t.Errorf("LastRunAt = %v, want %v", got.LastRunAt, at)
	}

	// Update scoped by server id: a mismatched server must not touch it.
	got.ServerID = serverID + 999
	got.TimeOfDay = "06:00"
	if err := st.UpdateRestartSchedule(ctx, got); err != ErrNotFound {
		t.Errorf("cross-server update: err = %v, want ErrNotFound", err)
	}

	if err := st.DeleteRestartSchedule(ctx, serverID, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetRestartSchedule(ctx, id); err != ErrNotFound {
		t.Errorf("get after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDiscordWebhookEncryptedAtRest(t *testing.T) {
	st, sqlDB := newTestStore(t)
	ctx := context.Background()
	serverID := testServerID(t, st)
	const url = "https://discord.com/api/webhooks/123/secret-token"

	err := st.SetDiscordWebhook(ctx, &DiscordWebhook{
		ServerID: serverID, WebhookURL: url, Enabled: true, OnStatus: true,
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := st.GetDiscordWebhook(ctx, serverID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.WebhookURL != url || !got.Enabled || !got.OnStatus || got.OnPlayers || got.OnRestarts {
		t.Errorf("round-trip mismatch: %+v", got)
	}

	var raw string
	if err := sqlDB.QueryRow(`SELECT webhook_url_enc FROM discord_webhooks WHERE server_id = ?`, serverID).Scan(&raw); err != nil {
		t.Fatalf("raw read: %v", err)
	}
	if strings.Contains(raw, "secret-token") {
		t.Error("webhook URL stored in plaintext")
	}

	// Upsert with a blank URL keeps the stored secret but updates toggles.
	err = st.SetDiscordWebhook(ctx, &DiscordWebhook{ServerID: serverID, Enabled: false, OnPlayers: true})
	if err != nil {
		t.Fatalf("toggle-only set: %v", err)
	}
	got, err = st.GetDiscordWebhook(ctx, serverID)
	if err != nil {
		t.Fatalf("get after toggle: %v", err)
	}
	if got.WebhookURL != url || got.Enabled || !got.OnPlayers {
		t.Errorf("toggle-only update mismatch: %+v", got)
	}

	// With nothing stored, a blank URL is an error, not an empty secret.
	other := testServerID(t, st)
	if err := st.SetDiscordWebhook(ctx, &DiscordWebhook{ServerID: other}); err == nil {
		t.Error("blank URL with nothing stored should fail")
	}

	if err := st.DeleteDiscordWebhook(ctx, serverID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.GetDiscordWebhook(ctx, serverID); err != ErrNotFound {
		t.Errorf("get after delete: err = %v, want ErrNotFound", err)
	}
}
