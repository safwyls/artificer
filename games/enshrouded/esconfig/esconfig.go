// Package esconfig reads and edits enshrouded_server.json — the one file
// that is all of Enshrouded's server administration.
//
// The policy is inherited from palcon's ini editors and matters more than
// the format: never remove or reorder what the game wrote, validate before
// writing, keep one .bak, swap atomically. JSON makes the parsing trivial
// but the stakes identical — a malformed file and the server boots on
// defaults (open, password-less), which is worse than not booting.
//
// The schema knowledge here is deliberately shallow. The full gameSettings
// table lives in games/enshrouded/docs/recon.md and changes with game updates;
// this package only understands the handful of keys the agent enforces
// (name, queryPort, the role-group passwords) and passes everything else
// through untouched, so a game update that adds keys costs no code here.
package esconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Doc is a parsed enshrouded_server.json. It is a plain map on purpose:
// the game owns the schema and this editor must survive keys it has never
// heard of.
type Doc map[string]any

// Parse decodes a config file's bytes. json.Number keeps 64-bit duration
// values (nanosecond int64s well past 2^53) intact through a round trip —
// float64 would silently corrupt dayTimeDuration and friends.
func Parse(data []byte) (Doc, error) {
	var doc Doc
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("enshrouded_server.json: %w", err)
	}
	return doc, nil
}

// Load reads and parses the config at path.
func Load(path string) (Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Marshal renders the doc as the indented JSON the game itself writes.
func (d Doc) Marshal() ([]byte, error) {
	out, err := json.MarshalIndent(d, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// Write validates and atomically replaces the config at path, keeping one
// .bak of the previous contents. The temp-then-rename is what guarantees
// the game can never boot on a half-written file.
func Write(path string, doc Doc) error {
	out, err := doc.Marshal()
	if err != nil {
		return err
	}
	if prev, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(path+".bak", prev, 0o644)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Enforcement is what the agent keeps authoritative on every game start:
// the identity settings the operator configured through the console.
// Nil string pointers mean "leave whatever is there alone" — an unset
// password env must not blank a password an operator set by hand.
type Enforcement struct {
	// ServerName is the server-browser name (the json "name").
	ServerName string
	// QueryPort is the single UDP port; zero leaves the file's value.
	QueryPort int
	// AdminPassword is enforced onto the first kick/ban-capable group
	// (seeding one when the file has none).
	AdminPassword *string
	// JoinPassword is enforced onto the first non-admin group (seeding one
	// when the file has none). An empty non-nil value makes the server
	// open — that is Enshrouded's own semantics for a blank password.
	JoinPassword *string
}

// adminGroup and joinGroup are the groups the agent seeds when a config
// has none. Names are plain on purpose — they surface in the game's own
// permission model, not in Flametender's theme.
func adminGroup(password string) map[string]any {
	return map[string]any{
		"name":                 "Admins",
		"password":             password,
		"canKickBan":           true,
		"canAccessInventories": true,
		"canEditBase":          true,
		"canExtendBase":        true,
		"canEditWorld":         true,
		"reservedSlots":        json.Number("1"),
	}
}

func joinGroup(password string) map[string]any {
	return map[string]any{
		"name":                 "Friends",
		"password":             password,
		"canKickBan":           false,
		"canAccessInventories": true,
		"canEditBase":          true,
		"canExtendBase":        true,
		"canEditWorld":         true,
		"reservedSlots":        json.Number("0"),
	}
}

// Enforce applies e to doc, reporting whether anything changed. Groups are
// matched by capability, not name: the first canKickBan group is the admin
// group, the first without it is the join group — names belong to the
// operator and renaming one must not orphan its password enforcement.
func Enforce(doc Doc, e Enforcement) bool {
	changed := false
	if e.ServerName != "" && doc["name"] != e.ServerName {
		doc["name"] = e.ServerName
		changed = true
	}
	if e.QueryPort > 0 {
		if n, ok := doc["queryPort"].(json.Number); !ok || n.String() != fmt.Sprint(e.QueryPort) {
			doc["queryPort"] = json.Number(fmt.Sprint(e.QueryPort))
			changed = true
		}
	}

	if e.AdminPassword == nil && e.JoinPassword == nil {
		return changed
	}
	groups, _ := doc["userGroups"].([]any)
	setPassword := func(admin bool, password string) {
		for _, g := range groups {
			m, ok := g.(map[string]any)
			if !ok {
				continue
			}
			kickBan, _ := m["canKickBan"].(bool)
			if kickBan != admin {
				continue
			}
			if m["password"] != password {
				m["password"] = password
				changed = true
			}
			return
		}
		// No matching group: seed one rather than leave the credential the
		// operator configured silently unenforced.
		if admin {
			groups = append([]any{adminGroup(password)}, groups...)
		} else {
			groups = append(groups, joinGroup(password))
		}
		changed = true
	}
	if e.AdminPassword != nil {
		setPassword(true, *e.AdminPassword)
	}
	if e.JoinPassword != nil {
		setPassword(false, *e.JoinPassword)
	}
	doc["userGroups"] = groups
	return changed
}

// Seed builds a complete first-boot config. The game would generate its
// own if the file were absent — but that default is an *open* server named
// "Enshrouded Server", so seeding before first boot is what makes a
// provisioned server private from its first second online. Keys and
// defaults follow the recon doc; save/log directories stay the game's own
// relative defaults so they land inside the install volume.
func Seed(e Enforcement) Doc {
	name := e.ServerName
	if name == "" {
		name = "Enshrouded Server"
	}
	port := e.QueryPort
	if port <= 0 {
		port = 15637
	}
	adminPW := ""
	if e.AdminPassword != nil {
		adminPW = *e.AdminPassword
	}
	joinPW := ""
	if e.JoinPassword != nil {
		joinPW = *e.JoinPassword
	}
	return Doc{
		"name":               name,
		"saveDirectory":      "./savegame",
		"logDirectory":       "./logs",
		"ip":                 "0.0.0.0",
		"queryPort":          json.Number(fmt.Sprint(port)),
		"slotCount":          json.Number("16"),
		"voiceChatMode":      "Proximity",
		"enableVoiceChat":    false,
		"enableTextChat":     false,
		"gameSettingsPreset": "Default",
		"userGroups":         []any{adminGroup(adminPW), joinGroup(joinPW)},
	}
}

// Validate is what the config editor runs before accepting an upload: it
// must parse, and the fields whose corruption bricks or exposes the server
// must have sane shapes. Everything else is the operator's business.
func Validate(data []byte) error {
	doc, err := Parse(data)
	if err != nil {
		return err
	}
	if port, ok := doc["queryPort"]; ok {
		n, ok := port.(json.Number)
		if !ok {
			return errors.New("queryPort must be a number")
		}
		v, err := n.Int64()
		if err != nil || v < 1 || v > 65535 {
			return errors.New("queryPort must be in 1-65535")
		}
	}
	if slots, ok := doc["slotCount"]; ok {
		n, ok := slots.(json.Number)
		if !ok {
			return errors.New("slotCount must be a number")
		}
		v, err := n.Int64()
		if err != nil || v < 1 || v > 16 {
			return errors.New("slotCount must be in 1-16")
		}
	}
	if groups, ok := doc["userGroups"]; ok {
		if _, ok := groups.([]any); !ok {
			return errors.New("userGroups must be a list of role groups")
		}
	}
	if bans, ok := doc["bannedAccounts"]; ok {
		if _, ok := bans.([]any); !ok {
			return errors.New("bannedAccounts must be a list")
		}
	}
	return nil
}
