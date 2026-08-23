package main

// The read-only rules the UI shares with the web frontend: how a game is
// identified, which custody state a linked world is in, what starts it,
// and how a timestamp is said out loud.
//
// Every function here is a port of its counterpart in
// web/companion/src/lib (types.ts, format.ts), rule for rule. They are
// *labelling* rules — none of them decides anything; the engine does
// that, and these only name what it decided. That is the line this file
// must not cross: a rule here that the engine does not also enforce is a
// second implementation of custody, which is exactly what the engine
// extraction exists to prevent.

import (
	"fmt"
	"strings"
	"time"

	"github.com/safwyls/artificer/companion"
)

// custody is the state a linked world is in, as the chip and the row's
// primary action both read it.
type custody string

const (
	custodyFree     custody = "free"
	custodyMine     custody = "mine"
	custodyFetching custody = "fetching"
	custodyHeld     custody = "held"
	custodyExpired  custody = "expired"
	custodyGone     custody = "gone"
)

// gameKey is the identity a game has everywhere: the artwork map's key,
// the hidden list's key, and what the service is asked about. One game,
// one key, in all three places.
func gameKey(appID, name string) string {
	if appID != "" {
		return "app:" + appID
	}
	return "name:" + strings.ToLower(strings.TrimSpace(name))
}

// custodyOf reads a link and the service's word about its world into one
// state.
//
// The subtle one is "fetching": the service says this account holds the
// world, but this machine has no session for it — another machine of
// theirs took it, or a queued claim's download is still on its way here.
// It offers no verbs, because the save is not here yet.
func custodyOf(link companion.WorldLink, world *companion.World, me string, configured bool) custody {
	if world == nil {
		if configured {
			return custodyGone
		}
		return custodyFree
	}
	h := world.Holder
	if h == nil {
		return custodyFree
	}
	if h.Username == me {
		if link.SessionID == h.SessionID {
			return custodyMine
		}
		return custodyFetching
	}
	if h.Claimable {
		return custodyExpired
	}
	return custodyHeld
}

// launchTargetOf is what the companion will open to start this world's
// game, or "" when it has nothing to start.
//
// Mirrors launchTarget() in the engine's launch.go. The engine is still
// the one that decides; this only labels the button, and a folder linked
// by hand carries no app id, so the companion must not pretend.
func launchTargetOf(link companion.WorldLink) string {
	if t := strings.TrimSpace(link.LaunchTarget); t != "" {
		return t
	}
	if link.AppID != "" {
		return "steam://rungameid/" + link.AppID
	}
	return ""
}

func launchable(link companion.WorldLink) bool { return launchTargetOf(link) != "" }

// worldFor finds the service's word about a linked world, or nil when
// the world is no longer there.
func worldFor(st companion.State, worldID int64) *companion.World {
	for i := range st.Sync.Worlds {
		if st.Sync.Worlds[i].World.ID == worldID {
			return &st.Sync.Worlds[i]
		}
	}
	return nil
}

// linkFor finds the link covering a discovered game: by the title
// recorded when linking, else by a save folder matching one of its
// candidates. A game linked before app ids were recorded still matches.
func linkFor(g companion.Game, links []companion.WorldLink) *companion.WorldLink {
	for i := range links {
		l := &links[i]
		if l.GameTitle != "" && l.GameTitle == g.Name {
			return l
		}
		for _, c := range g.SaveDirs {
			if c.Path == l.Dir {
				return l
			}
		}
	}
	return nil
}

// custodyLine is the sentence under the world's name: what the state
// means for the player standing in front of it.
func custodyLine(c custody, link companion.WorldLink, world *companion.World, me string) string {
	var h *companion.Holder
	next := ""
	if world != nil {
		h = world.Holder
		if world.ClaimedBy != "" {
			if world.ClaimedBy == me {
				next = " · you're next"
			} else {
				next = " · next claim: " + world.ClaimedBy
			}
		}
	}
	switch c {
	case custodyGone:
		return fmt.Sprintf("world #%d is not on the service any more", link.WorldID)
	case custodyFree:
		return "nobody holds this world" + next
	case custodyMine:
		return "until " + fmtTime(holderExpiry(h)) + " · save is on this machine"
	case custodyFetching:
		return "fetching it to this machine…"
	case custodyExpired:
		return "held by " + holderName(h) + " — the hold expired" + next
	default:
		return "held by " + holderName(h) + " until " + fmtTime(holderExpiry(h)) + next
	}
}

func holderName(h *companion.Holder) string {
	if h == nil {
		return "someone"
	}
	return h.Username
}

func holderExpiry(h *companion.Holder) time.Time {
	if h == nil {
		return time.Time{}
	}
	return h.ExpiresAt
}

// fmtTime says a moment in the player's own locale and zone — a hold
// expiring is a question about *their* clock.
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2 Jan 15:04")
}

// freshness is the age of what is on screen. Custody is shared state —
// someone else checking a world in is the whole reason this app exists —
// so "when did we last hear from the service" is worth saying rather
// than leaving people to guess.
func freshness(polledAt *time.Time) string {
	if polledAt == nil {
		return "not synced yet"
	}
	secs := int(time.Since(*polledAt).Seconds())
	if secs < 0 {
		secs = 0
	}
	switch {
	case secs < 10:
		return "up to date"
	case secs < 90:
		return fmt.Sprintf("synced %ds ago", secs)
	default:
		return fmt.Sprintf("synced %d min ago", (secs+30)/60)
	}
}

// plural counts both halves of a scan trail's summary.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
