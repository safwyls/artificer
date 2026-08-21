package main

// Hiding shelf entries.
//
// A Steam library is not a list of games. It also holds
// redistributables, driver bundles, controller configs and compatibility
// runtimes — installed like games, listed like games, and worth nothing
// on a shelf whose whole job is "click the game you want to sync".
// Discovery cannot tell them apart from the manifest alone (they are
// ordinary apps with ordinary install dirs), so the shelf lets the
// player put anything away, and starts with the usual suspects already
// away.
//
// Hidden is a view, not a filter on discovery: the scan still finds
// them, the state still carries them flagged, and unhiding needs no
// rescan.

import "strings"

// defaultHiddenAppIDs are Steam's own non-game apps, by app id — the
// exact ones, no guessing. These install into every library that has
// ever run a game that needed them.
var defaultHiddenAppIDs = map[string]bool{
	"228980":  true, // Steamworks Common Redistributables
	"241100":  true, // Steam Controller Configs
	"1070560": true, // Steam Linux Runtime 1.0 (scout)
	"1391110": true, // Steam Linux Runtime 2.0 (soldier)
	"1628350": true, // Steam Linux Runtime 3.0 (sniper)
	"1493710": true, // Proton Experimental
}

// defaultHiddenPrefixes catch the same class by name, for the versioned
// families whose app ids change with every release (Proton ships a new
// one a few times a year) and for libraries whose manifests are missing
// so only the folder name is known.
var defaultHiddenPrefixes = []string{
	"steamworks common",
	"steam controller config",
	"steam linux runtime",
	"proton ",
	"proton experimental",
	"proton hotfix",
	"steamvr",
}

// gameKey addresses a discovered game in the hidden list and the artwork
// map alike: the app id when there is one, else the lowercased name.
// Keeping the two keyed the same means the page can look a game up in
// either without a second identity to reconcile.
func gameKey(g discoveredGame) string {
	return artKey(artQuery{AppID: g.AppID, Name: g.Name})
}

// hiddenByDefault reports the entries that start out put away. The
// player's own list overrides it in both directions — an explicit
// "show" wins over a default, which is why this answers a question
// rather than editing the config at startup.
func hiddenByDefault(g discoveredGame) bool {
	if g.AppID != "" && defaultHiddenAppIDs[g.AppID] {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(g.Name))
	for _, prefix := range defaultHiddenPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// isHidden resolves one game against the config: an explicit entry wins,
// then the defaults. Shown-explicitly is recorded as "!key" so that
// unhiding a default sticks.
func (c Config) isHidden(g discoveredGame) bool {
	key := gameKey(g)
	for _, h := range c.Hidden {
		switch h {
		case key:
			return true
		case "!" + key:
			return false
		}
	}
	return hiddenByDefault(g)
}

// setHidden records a decision, replacing any previous one for the same
// entry. Both directions are stored explicitly: "hide this" and "no,
// really show this one" are equally the player's call.
func (c *Config) setHidden(key string, hidden bool) {
	kept := c.Hidden[:0]
	for _, h := range c.Hidden {
		if h != key && h != "!"+key {
			kept = append(kept, h)
		}
	}
	c.Hidden = kept
	if hidden {
		c.Hidden = append(c.Hidden, key)
	} else {
		c.Hidden = append(c.Hidden, "!"+key)
	}
}
