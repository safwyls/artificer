package esconfig

import (
	"errors"
	"fmt"
	"strings"
)

// The ban list.
//
// `bannedAccounts` is where the in-game kick/ban UI persists its bans, and
// the only ban surface this console can reach at all — Enshrouded has no
// RCON and no admin API, so every moderation command here 501s and points
// at the game's own player list (games/enshrouded/docs/recon.md). What the file
// gives us is the durable half: the list survives restarts, and it is
// editable.
//
// The element format is the recon doc's open ledger row: bare SteamID64
// strings is what the code and community both read it as, but nobody has
// confirmed it against a file a real server wrote with a real ban in it.
// So this editor never imposes a shape. It reads whichever of the two
// plausible ones the file already uses, writes new entries in that same
// shape, and preserves anything it can't model rather than dropping it —
// deleting a ban by accident is exactly the failure worth engineering
// against.

// idKeys and nameKeys are the object-shaped element's plausible fields, in
// the order they're tried. Used only when a file turns out to hold objects;
// a file holding strings never consults them.
var idKeys = []string{"id", "steamId", "steamID", "steamid", "accountId", "accountID", "userId", "playerId"}

var nameKeys = []string{"name", "playerName", "userName", "displayName"}

// Ban is one entry of bannedAccounts.
//
// Index is its position in the file as read, so a write can update the
// original element and keep any fields this struct doesn't model. A newly
// added ban carries -1.
type Ban struct {
	Index int `json:"index"`
	// ID is the account the game bans by — a 17-digit SteamID64 in every
	// example we have.
	ID string `json:"id"`
	// Name is a display name, present only when the file's elements are
	// objects that carry one. Never invented: an empty name means the file
	// doesn't hold one, not that the player is nameless.
	Name string `json:"name,omitempty"`
}

// BanList is what ReadBans returns.
type BanList struct {
	Bans     []Ban  `json:"bans"`
	Path     string `json:"path"`
	Writable bool   `json:"writable"`
	// ObjectShape reports that this file's entries are objects rather than
	// bare id strings — the ledger's open question, answered by the one
	// file that can answer it. The console shows it so the first real
	// server to carry a ban settles the row.
	ObjectShape bool `json:"objectShape"`
	// Unreadable counts entries this editor could not model. They are
	// preserved verbatim on write; the count exists so the console can say
	// so instead of appearing to have lost them.
	Unreadable int `json:"unreadable"`
}

// banShape is how a given file writes its entries.
type banShape struct {
	object  bool
	idKey   string
	nameKey string
}

// looksLikeAccountID is the same loose test validation uses: a run of
// digits about the length of a SteamID64. Loose on purpose — the id space
// is a ledger row, and a check tight enough to demand the 7656119 prefix
// would reject a legitimate entry if the game turns out to ban by
// something else.
func looksLikeAccountID(s string) bool {
	if len(s) < 15 || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// detectShape reads the file's own convention off its first usable entry.
// An empty or absent list has no convention to read, so it gets bare
// strings — the reading both the community tooling and mornedhels' config
// docs assume, and the one that round-trips into either interpretation if
// the game rewrites the list itself.
func detectShape(raw []any) banShape {
	for _, e := range raw {
		switch v := e.(type) {
		case string:
			return banShape{}
		case map[string]any:
			shape := banShape{object: true, idKey: "id"}
			for _, k := range idKeys {
				if s, ok := v[k].(string); ok && s != "" {
					shape.idKey = k
					break
				}
			}
			for _, k := range nameKeys {
				if _, ok := v[k].(string); ok {
					shape.nameKey = k
					break
				}
			}
			return shape
		}
	}
	return banShape{}
}

// banFrom reads one element under shape; ok=false means "not something
// this editor models", which the caller preserves untouched.
func banFrom(e any, index int, shape banShape) (Ban, bool) {
	switch v := e.(type) {
	case string:
		if v == "" {
			return Ban{}, false
		}
		return Ban{Index: index, ID: v}, true
	case map[string]any:
		id, _ := v[shape.idKey].(string)
		if id == "" {
			// The shape came off a different element; fall back to any field
			// that reads like an account id before giving up.
			for _, k := range idKeys {
				if s, ok := v[k].(string); ok && looksLikeAccountID(s) {
					id = s
					break
				}
			}
		}
		if id == "" {
			return Ban{}, false
		}
		name := ""
		if shape.nameKey != "" {
			name, _ = v[shape.nameKey].(string)
		}
		return Ban{Index: index, ID: id, Name: name}, true
	}
	return Ban{}, false
}

// BansFrom lifts bannedAccounts out of a parsed doc, reporting the file's
// element shape and how many entries it could not read.
func BansFrom(doc Doc) (bans []Ban, shape banShape, unreadable int) {
	raw, _ := doc["bannedAccounts"].([]any)
	shape = detectShape(raw)
	bans = make([]Ban, 0, len(raw))
	for i, e := range raw {
		b, ok := banFrom(e, i, shape)
		if !ok {
			unreadable++
			continue
		}
		bans = append(bans, b)
	}
	return bans, shape, unreadable
}

// ReadBans returns the ban list under configPath.
func ReadBans(configPath string) (*BanList, error) {
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
	bans, shape, unreadable := BansFrom(doc)
	return &BanList{
		Bans:        bans,
		Path:        file,
		Writable:    writable(file),
		ObjectShape: shape.object,
		Unreadable:  unreadable,
	}, nil
}

// ValidateBans checks the ids are plausible and unique. The id is the
// whole of a ban: a typo doesn't fail, it bans nobody, and the operator
// finds out when the player they meant to ban walks back in.
func ValidateBans(bans []Ban) error {
	seen := map[string]bool{}
	for _, b := range bans {
		id := strings.TrimSpace(b.ID)
		if id == "" {
			return errors.New("a ban needs an account id")
		}
		if !looksLikeAccountID(id) {
			return fmt.Errorf("%q doesn't look like a SteamID64 — it should be the player's 17-digit account id, not their name or profile URL", id)
		}
		if seen[id] {
			return fmt.Errorf("%s is in the list twice", id)
		}
		seen[id] = true
	}
	return nil
}

// SetBans replaces doc's bannedAccounts with bans, written in the file's
// own element shape. Entries the reader couldn't model are appended
// unchanged: they are somebody's ban, and this editor not understanding
// them is not a reason to lift them.
func SetBans(doc Doc, bans []Ban) {
	existing, _ := doc["bannedAccounts"].([]any)
	shape := detectShape(existing)

	out := make([]any, 0, len(bans))
	for _, b := range bans {
		id := strings.TrimSpace(b.ID)
		if !shape.object {
			out = append(out, id)
			continue
		}
		m := map[string]any{}
		if b.Index >= 0 && b.Index < len(existing) {
			// Copied, not aliased: two entries pointing at one index would
			// otherwise share a map and collapse into the same ban.
			if prev, ok := existing[b.Index].(map[string]any); ok {
				for k, v := range prev {
					m[k] = v
				}
			}
		}
		m[shape.idKey] = id
		if shape.nameKey != "" && b.Name != "" {
			m[shape.nameKey] = b.Name
		}
		out = append(out, m)
	}
	for i, e := range existing {
		if _, ok := banFrom(e, i, shape); !ok {
			out = append(out, e)
		}
	}
	doc["bannedAccounts"] = out
}

// WriteBans validates and saves the ban list under configPath.
//
// Two things the caller has to tell the user, because this function
// cannot: the game reads bannedAccounts at start, so an addition takes
// effect on the next restart and won't eject anyone already playing; and
// the running server owns this list too — the in-game ban UI writes it —
// so editing the file under a live server risks the game overwriting the
// edit when it next persists. Editing while stopped is the safe order.
func WriteBans(configPath string, bans []Ban) error {
	if configPath == "" {
		return ErrNotConfigured
	}
	if err := ValidateBans(bans); err != nil {
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
	SetBans(doc, bans)
	return Write(file, doc)
}
