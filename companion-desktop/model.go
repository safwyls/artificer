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

// custodyInfo is the single answer this UI is allowed to ask about a
// world's custody: which state it is in, who has it, and until when.
//
// The design's main invariant lives here. The chip and the row's primary
// action are both computed from one of these values and from nothing
// else, so the two cannot disagree — a row can never show "Free" beside
// a "Check in" button, because there is no second source either of them
// could read.
type custodyInfo struct {
	State     custody
	Holder    string
	ExpiresAt time.Time
	// NextClaim is who is queued behind the current holder, and
	// NextIsMe says that queue position is this account's.
	NextClaim string
	NextIsMe  bool
}

// group is which of the page's three sections a world belongs in.
// Derived from State alone — the sections *are* the custody states,
// which is what lets the grouping replace the per-row status prose.
type group int

const (
	groupMine group = iota
	groupFree
	groupHeld
	groupGone
)

func (c custodyInfo) group() group {
	switch c.State {
	case custodyMine, custodyFetching:
		return groupMine
	case custodyFree:
		return groupFree
	case custodyGone:
		return groupGone
	default:
		return groupHeld
	}
}

// holdLeft is how long is left on the hold, or zero when nothing is
// held or the moment has passed.
func (c custodyInfo) holdLeft(now time.Time) time.Duration {
	if c.ExpiresAt.IsZero() {
		return 0
	}
	if d := c.ExpiresAt.Sub(now); d > 0 {
		return d
	}
	return 0
}

// urgent is the design's "hold pressure is shown only when it is
// pressure" rule: a countdown appears on a world someone else holds
// only inside the last three hours of a 48-hour hold.
const urgentWindow = 3 * time.Hour

func (c custodyInfo) urgent(now time.Time) bool {
	left := c.holdLeft(now)
	return left > 0 && left < urgentWindow
}

// custodyOf reads a link and the service's word about its world into one
// state.
//
// The subtle one is "fetching": the service says this account holds the
// world, but this machine has no session for it — another machine of
// theirs took it, or a queued claim's download is still on its way here.
// It offers no verbs, because the save is not here yet.
func custodyOf(link companion.WorldLink, world *companion.World, me string, configured bool) custodyInfo {
	out := custodyInfo{State: custodyFree}
	if world == nil {
		if configured {
			out.State = custodyGone
		}
		return out
	}
	if world.ClaimedBy != "" {
		out.NextClaim = world.ClaimedBy
		out.NextIsMe = world.ClaimedBy == me
	}
	h := world.Holder
	if h == nil {
		return out
	}
	out.Holder, out.ExpiresAt = h.Username, h.ExpiresAt
	switch {
	case h.Username == me && link.SessionID == h.SessionID:
		out.State = custodyMine
	case h.Username == me:
		out.State = custodyFetching
	case h.Claimable:
		out.State = custodyExpired
	default:
		out.State = custodyHeld
	}
	return out
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

// custodyLine is the short sentence beside the chip. It is deliberately
// *not* a restatement of the chip: the group heading and the chip
// already say which state the world is in, so this says the one thing
// they cannot — how long is left, or whose it is.
func custodyLine(c custodyInfo, link companion.WorldLink, offline bool, now time.Time) string {
	next := ""
	switch {
	case c.NextIsMe:
		next = " · you're next"
	case c.NextClaim != "":
		next = " · next in line: " + c.NextClaim
	}
	switch c.State {
	case custodyGone:
		return fmt.Sprintf("world #%d is not on the service any more", link.WorldID)
	case custodyFree:
		return "nobody holds this world" + next
	case custodyMine:
		if offline {
			// The whole point of the offline state: the hold does not
			// lapse because the vault stopped answering.
			return "the hold stands while you are offline" + holdSuffix(c, now)
		}
		return "yours" + holdSuffix(c, now)
	case custodyFetching:
		return "fetching it to this machine…"
	case custodyExpired:
		return "held by " + holderName(c) + " — the hold expired" + next
	default:
		return "held by " + holderName(c) + next
	}
}

// holdSuffix is the " — 47h left on the hold" tail, present only when
// the service actually told us when the hold ends.
func holdSuffix(c custodyInfo, now time.Time) string {
	left := c.holdLeft(now)
	if left <= 0 {
		return ""
	}
	return " — " + fmtDuration(left) + " left on the hold"
}

func holderName(c custodyInfo) string {
	if c.Holder == "" {
		return "someone"
	}
	return c.Holder
}

// fmtDuration says a remaining hold the way a person would: hours while
// there are hours, minutes once it is down to them.
func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return "under a minute"
	}
	mins := int(d.Minutes())
	h, m := mins/60, mins%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

// headMeta is the mono line on the right of a row: the version the
// service holds, how big it is, and when it was written.
//
// Every field is optional, because the service answers with what it
// has: a world nobody has ever pushed a save to has no head at all.
// Only the parts that exist are rendered, and a world with none renders
// nothing rather than a row of dashes.
func headMeta(world *companion.World, now time.Time) string {
	if world == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if v := world.World.HeadVersion; v != nil && *v > 0 {
		parts = append(parts, "v"+itoa(*v))
	}
	if world.Head != nil {
		if world.Head.Bytes > 0 {
			parts = append(parts, fmtBytes(world.Head.Bytes))
		}
		if !world.Head.CreatedAt.IsZero() {
			parts = append(parts, fmtWhen(world.Head.CreatedAt, now))
		}
	}
	return strings.Join(parts, " · ")
}

// fmtBytes is a save's size at the precision anyone cares about.
func fmtBytes(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", (n+(1<<19))/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", (n+(1<<9))/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// fmtWhen says when a save was written, at the resolution that is
// useful: a clock time today, a weekday this week, a date beyond that.
func fmtWhen(t time.Time, now time.Time) string {
	t, now = t.Local(), now.Local()
	switch d := now.Sub(t); {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case sameDay(t, now):
		return t.Format("15:04")
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "yesterday"
	case d < 7*24*time.Hour:
		return t.Format("Monday")
	default:
		return t.Format("2 Jan")
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
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

// offline is the connectivity state the whole window keys off, derived
// rather than stored: the companion is configured, so it should be able
// to reach the vault, and the last poll says it could not.
//
// The engine has no offline flag — a failed poll leaving its error
// behind is the only signal there is, which is exactly what this reads.
func offline(st companion.State) bool {
	return st.Sync.Configured && st.Sync.LastError != ""
}

// worldsOf splits the linked worlds into the page's sections, in the
// order they are shown. One custodyOf call per world, and the section
// it lands in comes from that call and nothing else.
func worldsOf(st companion.State, now time.Time) map[group][]worldRowData {
	out := map[group][]worldRowData{}
	for _, link := range st.Links {
		w := worldFor(st, link.WorldID)
		c := custodyOf(link, w, st.Sync.Username, st.Sync.Configured)
		out[c.group()] = append(out[c.group()], worldRowData{
			link: link, world: w, custody: c,
		})
	}
	return out
}

// worldRowData is one row's inputs, resolved once so the row builder
// cannot go back and ask a second time.
type worldRowData struct {
	link    companion.WorldLink
	world   *companion.World
	custody custodyInfo
}

func (d worldRowData) name() string {
	if d.world != nil && d.world.World.Name != "" {
		return d.world.World.Name
	}
	return "world #" + itoa(d.link.WorldID)
}

func (d worldRowData) gameTitle() string {
	if d.link.GameTitle != "" {
		return d.link.GameTitle
	}
	if d.world != nil {
		return d.world.World.GameTitle
	}
	return ""
}

// plural counts both halves of a scan trail's summary.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
