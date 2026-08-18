package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/safwyls/artificer/core/anvilclient"
	"github.com/safwyls/artificer/core/api"
)

// These drive the real Anvil client and adapter over an Anvil-shaped HTTP
// server, rather than a fake Provisioner handing the handler an error it
// invented. That distinction is the whole point: the handlers below branch
// on agentctl's sentinels, and the fakes elsewhere in this package produce
// those sentinels directly — so every one of them passed while the live
// Anvil path produced untyped errors and took none of the branches.
func anvilAt(t *testing.T, h http.HandlerFunc) api.Provisioner {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	client, err := anvilclient.New(srv.URL, "test-token-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return api.NewAnvilProvisioner(client, testProfile)
}

func registerServer(t *testing.T, app *testApp, admin []*http.Cookie, name, container string) string {
	t.Helper()
	rec := app.do(t, "POST", "/api/servers", map[string]any{
		"name": name, "host": "10.0.0.9", "enabled": true, "containerName": container,
	}, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create server: %d (body %s)", rec.Code, rec.Body)
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	return itoa(created.ID)
}

// Someone removed the container by hand. The end state is the one the
// operator is asking for, so the row goes — otherwise they are trapped in
// a retry that can never succeed, with a card for a server that does not
// exist and no way to delete it.
func TestDeleteServerClearsTheRowWhenAnvilSaysTheContainerIsGone(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Provisioner = anvilAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"no container with that name"}`))
	})

	id := registerServer(t, app, admin, "Ghost", "gtagent-ghost")
	rec := app.do(t, "DELETE", "/api/servers/"+id+"?removeContainer=true", nil, admin)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("destroying an already-gone container: %d (body %s), want 204", rec.Code, rec.Body)
	}
	if rec := app.do(t, "GET", "/api/servers/"+id, nil, admin); rec.Code != http.StatusNotFound {
		t.Errorf("row survived: %d — the operator cannot get rid of this server", rec.Code)
	}
}

// The opposite answer must produce the opposite outcome. A refusal leaves
// the container standing, so deleting the row would strand it: still
// running, still holding ports, with its credentials only in the row that
// was just dropped.
func TestDeleteServerKeepsTheRowWhenAnvilRefuses(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Provisioner = anvilAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"that container belongs to a different console"}`))
	})

	id := registerServer(t, app, admin, "Not Mine", "palagent-palhalla")
	rec := app.do(t, "DELETE", "/api/servers/"+id+"?removeContainer=true", nil, admin)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("refused destroy: %d (body %s), want 400", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "different console") {
		t.Errorf("the operator is not told why: %s", rec.Body)
	}
	if rec := app.do(t, "GET", "/api/servers/"+id, nil, admin); rec.Code != http.StatusOK {
		t.Errorf("row deleted despite the refusal: %d", rec.Code)
	}
}

// A destroy that fails because Anvil itself is broken is neither. Deleting
// the row here would discard a live server's registration on the strength
// of an answer nobody gave.
func TestDeleteServerKeepsTheRowWhenAnvilIsBroken(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Provisioner = anvilAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":"docker endpoint unreachable"}`))
	})

	id := registerServer(t, app, admin, "Live", "gtagent-live")
	if rec := app.do(t, "DELETE", "/api/servers/"+id+"?removeContainer=true", nil, admin); rec.Code != http.StatusBadGateway {
		t.Fatalf("destroy against a broken anvil: %d (body %s), want 502", rec.Code, rec.Body)
	}
	if rec := app.do(t, "GET", "/api/servers/"+id, nil, admin); rec.Code != http.StatusOK {
		t.Errorf("row deleted on an inconclusive answer: %d", rec.Code)
	}
}

// A conflict Anvil reports is fatal to the provision: nothing was created,
// so a row would describe a server that does not exist. Discover cannot
// pre-empt every one of these — it is scoped to this console's own agent
// family, and the container in the way may be another console's or
// something deployed by hand — which is exactly when this branch runs.
func TestProvisionRegistersNothingWhenAnvilReportsAConflict(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantSubstr string
	}{
		{
			name:       "the name is taken",
			body:       `{"error":"a container named gtagent-taken already exists on this host","reason":"name-taken"}`,
			wantSubstr: "adopt the existing container",
		},
		{
			// The name-taken advice would be wrong here — there is nothing
			// to adopt, the port belongs to something else entirely.
			name:       "a port is held by another console",
			body:       `{"error":"host port already in use: 25600 (palagent-palhalla)","reason":"ports-in-use"}`,
			wantSubstr: "palagent-palhalla",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, admin := newTestAppWithAdmin(t)
			app.api.Provision = testProfile
			app.api.Provisioner = anvilAt(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/health":
					json.NewEncoder(w).Encode(map[string]any{
						"service": "anvil", "apiVersion": 1, "client": "test",
						"dataRoot": "/mnt/pool/apps", "dockerOk": true,
					})
				case "/v1/discover":
					// Blind to it: the container in the way is not this
					// console's family, so the pre-check cannot help.
					json.NewEncoder(w).Encode(map[string]any{"servers": []any{}})
				case "/v1/provision":
					w.WriteHeader(http.StatusConflict)
					w.Write([]byte(tc.body))
				default:
					w.Write([]byte(`{}`))
				}
			})

			rec := app.do(t, "POST", "/api/servers/provision", map[string]any{
				"name": "Taken", "host": "10.0.0.9", "dataPath": "/mnt/pool/apps/taken",
			}, admin)
			if rec.Code != http.StatusConflict {
				t.Fatalf("provision against a conflict: %d, want 409 (body %s)", rec.Code, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), tc.wantSubstr) {
				t.Errorf("message %s should contain %q — the two conflicts need different advice", rec.Body, tc.wantSubstr)
			}
			servers, err := app.store.ListServers(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(servers) != 0 {
				t.Errorf("a refused deploy registered %d server(s): %+v", len(servers), servers)
			}
		})
	}
}

// The port proposal must count ports held by anything on the machine, not
// just this console's servers. Proposing one another console holds is the
// collision Anvil was built to end, and it comes back the moment the
// wizard stops asking.
func TestPortProposalAvoidsPortsAnotherConsoleHolds(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Provision = testProfile
	app.api.Provisioner = anvilAt(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/health":
			json.NewEncoder(w).Encode(map[string]any{
				"service": "anvil", "apiVersion": 1, "client": "test",
				"dataRoot": "/mnt/pool/apps", "publicHost": "10.0.0.9", "dockerOk": true,
			})
		case "/v1/discover":
			// This console owns nothing here — its own view is empty.
			json.NewEncoder(w).Encode(map[string]any{"servers": []any{}})
		case "/v1/ports":
			// But the default game port and the default agent port are
			// both already published, by containers it cannot see.
			json.NewEncoder(w).Encode(map[string]any{"ports": []map[string]any{
				{"port": 25600, "proto": "udp", "container": "palagent-palhalla"},
				{"port": 8811, "proto": "tcp", "container": "palagent-palhalla"},
			}})
		default:
			w.Write([]byte(`{}`))
		}
	})

	rec := app.do(t, "GET", "/api/servers/provision/defaults", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("defaults: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Available bool           `json:"available"`
		Ports     map[string]int `json:"ports"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Available {
		t.Fatalf("wizard unavailable: %s", rec.Body)
	}
	if res.Ports["game"] == 25600 {
		t.Errorf("proposed game port %d, which palcon already holds", res.Ports["game"])
	}
	if res.Ports["agent"] == 8811 {
		t.Errorf("proposed agent port %d, which palcon already holds", res.Ports["agent"])
	}
}

// Anvil going quiet must not empty the wizard. The proposal is a
// suggestion either way, and slightly worse ports beat no form at all.
func TestPortProposalSurvivesAnvilNotAnsweringPorts(t *testing.T) {
	app, admin := newTestAppWithAdmin(t)
	app.api.Provision = testProfile
	app.api.Provisioner = anvilAt(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/health":
			json.NewEncoder(w).Encode(map[string]any{
				"service": "anvil", "apiVersion": 1, "client": "test",
				"dataRoot": "/mnt/pool/apps", "dockerOk": true,
			})
		case "/v1/ports":
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error":"docker endpoint unreachable"}`))
		default:
			json.NewEncoder(w).Encode(map[string]any{"servers": []any{}})
		}
	})

	rec := app.do(t, "GET", "/api/servers/provision/defaults", nil, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("defaults: %d (body %s)", rec.Code, rec.Body)
	}
	var res struct {
		Available bool           `json:"available"`
		Ports     map[string]int `json:"ports"`
	}
	json.Unmarshal(rec.Body.Bytes(), &res)
	if !res.Available || res.Ports["game"] == 0 {
		t.Errorf("a silent /v1/ports emptied the wizard: %s", rec.Body)
	}
}
