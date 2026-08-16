package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/safwyls/sampo/core/game"
)

// The status split is the contract the frontend reads: 501 means "this game
// cannot do this at all" (render as a capability gap), 502 means "a server
// that could do it didn't answer" (render as unreachable).
func TestWriteClientErrorStatusSplit(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		msg    string
	}{
		{
			name:   "unsupported op",
			err:    &game.UnsupportedError{Op: "broadcast", Reason: "no native console"},
			status: 501,
			msg:    "broadcast unsupported: no native console",
		},
		{
			name:   "wrapped unsupported op",
			err:    fmt.Errorf("dragonwilds: %w", &game.UnsupportedError{Op: "kick"}),
			status: 501,
			msg:    "kick is not supported by this game",
		},
		{
			name:   "transport failure",
			err:    errors.New("dial tcp: connection refused"),
			status: 502,
			msg:    "dial tcp: connection refused",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeClientError(rec, tc.err)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Error != tc.msg {
				t.Fatalf("error = %q, want %q", body.Error, tc.msg)
			}
		})
	}
}
