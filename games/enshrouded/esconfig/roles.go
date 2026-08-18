package esconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Role groups as a first-class model, rather than the flat scalar view the
// settings editor serves.
//
// `userGroups` is the whole of Enshrouded's permission system: a joining
// player types a group's password and gets that group's rights for the
// session (games/enshrouded/docs/recon.md, "Role groups"). That makes this list
// both the access-control policy *and* the credential store, which is why
// the editor reads and writes whole groups instead of exposing
// `userGroups[0].password` as another string setting.
//
// Two policies carry over from the flat editor and matter more here:
// unknown keys inside a group are preserved (the game has added fields
// twice already — canEditWorld arrived in 2025), and a write either
// validates whole or touches nothing.

// Group is one entry of userGroups.
//
// Index is the group's position in the file as read. It exists so a write
// can find the original element and keep any keys this struct doesn't
// know about; a group the operator just added carries -1 (or any
// out-of-range value) and is written from scratch.
type Group struct {
	Index                int    `json:"index"`
	Name                 string `json:"name"`
	Password             string `json:"password"`
	CanKickBan           bool   `json:"canKickBan"`
	CanAccessInventories bool   `json:"canAccessInventories"`
	CanEditBase          bool   `json:"canEditBase"`
	CanExtendBase        bool   `json:"canExtendBase"`
	CanEditWorld         bool   `json:"canEditWorld"`
	ReservedSlots        int    `json:"reservedSlots"`
}

// Groups is what ReadGroups returns: the list plus the same file context
// the flat editor reports, so the console can warn about a read-only
// mount before someone types a password into a form that can't save it.
type Groups struct {
	Groups   []Group `json:"groups"`
	Path     string  `json:"path"`
	Writable bool    `json:"writable"`
}

func boolAt(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

func intAt(m map[string]any, key string) int {
	n, ok := m[key].(json.Number)
	if !ok {
		return 0
	}
	v, err := n.Int64()
	if err != nil {
		return 0
	}
	return int(v)
}

// GroupsFrom lifts userGroups out of a parsed doc. Anything in the list
// that isn't an object is skipped rather than fatal — the file belongs to
// the game, and refusing to show the groups because a future version put
// something unexpected beside them would be the wrong trade.
func GroupsFrom(doc Doc) []Group {
	raw, _ := doc["userGroups"].([]any)
	out := make([]Group, 0, len(raw))
	for i, g := range raw {
		m, ok := g.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		password, _ := m["password"].(string)
		out = append(out, Group{
			Index:                i,
			Name:                 name,
			Password:             password,
			CanKickBan:           boolAt(m, "canKickBan"),
			CanAccessInventories: boolAt(m, "canAccessInventories"),
			CanEditBase:          boolAt(m, "canEditBase"),
			CanExtendBase:        boolAt(m, "canExtendBase"),
			CanEditWorld:         boolAt(m, "canEditWorld"),
			ReservedSlots:        intAt(m, "reservedSlots"),
		})
	}
	return out
}

// ReadGroups returns the role groups under configPath.
func ReadGroups(configPath string) (*Groups, error) {
	if configPath == "" {
		return nil, ErrNotConfigured
	}
	file, err := settingsFile(configPath)
	if err != nil {
		return nil, err
	}
	doc, err := Load(file)
	if err != nil {
		return nil, err
	}
	return &Groups{Groups: GroupsFrom(doc), Path: file, Writable: writable(file)}, nil
}

// ValidateGroups refuses the edits that would quietly break a server, and
// nothing else. The three are all cases where the file stays valid JSON
// and the game boots happily into a state the operator did not intend:
//
//   - a nameless group, which the join screen can't label;
//   - an admin group with a blank password, which is server-wide kick/ban
//     rights handed to anyone who finds the server;
//   - no admin group at all, which removes the *only* moderation this
//     deployment has — Enshrouded's kick/ban lives in the in-game player
//     list and nowhere else, so a config with no canKickBan group leaves
//     the console with no lever either.
//
// Duplicate passwords are refused too: the game matches a joining player
// to a group by password, so two groups sharing one makes which rights
// they get an implementation detail of the game's iteration order.
func ValidateGroups(groups []Group) error {
	if len(groups) == 0 {
		return errors.New("keep at least one role group — a server with none has no way for anyone to join")
	}
	admins := 0
	seenName := map[string]bool{}
	seenPassword := map[string]bool{}
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			return errors.New("every role group needs a name")
		}
		if seenName[strings.ToLower(name)] {
			return fmt.Errorf("two role groups are both called %q", name)
		}
		seenName[strings.ToLower(name)] = true
		if g.ReservedSlots < 0 || g.ReservedSlots > 16 {
			return fmt.Errorf("%s: reserved slots must be 0-16 (the server's hard cap)", name)
		}
		if g.CanKickBan {
			admins++
			if g.Password == "" {
				return fmt.Errorf("%s can kick and ban, so it must have a password — an empty one lets anyone join as an admin", name)
			}
		}
		if g.Password != "" {
			if seenPassword[g.Password] {
				return fmt.Errorf("%s shares its password with another group; the game picks a joining player's group by password, so they must differ", name)
			}
			seenPassword[g.Password] = true
		}
	}
	if admins == 0 {
		return errors.New("keep one group with kick/ban rights — moderation happens in the game's own player list, and without an admin group nobody can reach it")
	}
	return nil
}

// applyGroup writes g's fields onto dst, which is either the group's
// original map (keeping keys this editor doesn't model) or a fresh one.
func applyGroup(dst map[string]any, g Group) map[string]any {
	if dst == nil {
		dst = map[string]any{}
	}
	dst["name"] = strings.TrimSpace(g.Name)
	dst["password"] = g.Password
	dst["canKickBan"] = g.CanKickBan
	dst["canAccessInventories"] = g.CanAccessInventories
	dst["canEditBase"] = g.CanEditBase
	dst["canExtendBase"] = g.CanExtendBase
	dst["canEditWorld"] = g.CanEditWorld
	dst["reservedSlots"] = json.Number(fmt.Sprint(g.ReservedSlots))
	return dst
}

// SetGroups replaces doc's userGroups with groups, carrying each group's
// unmodelled keys across by Index. The resulting order is the caller's,
// not the file's — the console lets groups be reordered, and the game
// reads the list in order.
func SetGroups(doc Doc, groups []Group) {
	existing, _ := doc["userGroups"].([]any)
	out := make([]any, 0, len(groups))
	for _, g := range groups {
		var base map[string]any
		if g.Index >= 0 && g.Index < len(existing) {
			if m, ok := existing[g.Index].(map[string]any); ok {
				base = m
			}
		}
		out = append(out, applyGroup(base, g))
	}
	doc["userGroups"] = out
}

// WriteGroups validates and saves the role groups under configPath.
//
// The whole list is replaced, so a group the caller dropped is deleted —
// which is the point, and why validation runs first. Changes land on the
// next server start: the game reads userGroups at boot and checks a
// joining player against the copy it loaded then.
func WriteGroups(configPath string, groups []Group) error {
	if configPath == "" {
		return ErrNotConfigured
	}
	if err := ValidateGroups(groups); err != nil {
		return err
	}
	file, err := settingsFile(configPath)
	if err != nil {
		return err
	}
	doc, err := Load(file)
	if err != nil {
		return err
	}
	SetGroups(doc, groups)
	return Write(file, doc)
}
