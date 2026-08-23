package main

// OS notifications, for exactly three events.
//
// This is the whole list, and it is short on purpose. A companion that
// speaks up whenever anything changes is a companion people mute, and a
// muted companion cannot tell them the one thing that matters. So:
//
//  1. A queued claim came through — the world is checked out to this
//     machine now, and nobody asked for it at this moment. This is the
//     only event in the app that happens entirely without the player,
//     which is exactly what makes it worth an interruption.
//  2. A hold is nearing expiry — the window may not be open, and a hold
//     that lapses is somebody else's takeover.
//  3. A sync failed while the window was hidden. The header's error band
//     says it perfectly well when the window is up; hidden, nothing
//     would.
//
// Everything else stays in-window in the status strip. In particular:
// no notification for a checkout the player just clicked, none for a
// checkpoint, none for an update.

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"

	"github.com/safwyls/artificer/companion"
)

const (
	// holdWarnWithin is how close to expiry a hold has to be before it is
	// worth saying out loud.
	holdWarnWithin = 15 * time.Minute
	// holdWarnAgain bounds the repeat: a hold that stays near expiry
	// because nobody acted is mentioned once an hour, not every tick.
	holdWarnAgain = time.Hour
)

// notifyOff is the player's setting, stored beside the window state so
// the browser build's config file learns nothing new.
var notifyOff atomic.Bool

func (u *ui) notifyEnabled() bool { return !notifyOff.Load() }

func (u *ui) setNotifyEnabled(on bool) {
	notifyOff.Store(!on)
	u.saveNotifyPref(!on)
}

// notify sends one OS notification, if the player has not turned them
// off.
func (u *ui) notify(title, body string) {
	if !u.notifyEnabled() || u.app == nil {
		return
	}
	u.app.SendNotification(fyne.NewNotification(title, body))
}

// announce compares two snapshots and speaks up about the three events
// worth hearing with the window closed. Called on every engine nudge,
// off the UI thread.
func (u *ui) announce(prev, now companion.State) {
	u.claimArrived(prev, now)
	u.holdNearlyUp(now)
	u.syncFailedUnseen(now)
}

// claimArrived spots a hold this machine did not ask for. A link that
// had no session and now has one, with no checkout of ours in flight, is
// a queued claim that came through — the engine adopted the handoff and
// the save is here.
func (u *ui) claimArrived(prev, now companion.State) {
	before := map[int64]int64{}
	for _, l := range prev.Links {
		before[l.WorldID] = l.SessionID
	}
	for _, l := range now.Links {
		was, seen := before[l.WorldID]
		if !seen || was != 0 || l.SessionID == 0 {
			continue
		}
		if u.selfCheckout.Load() {
			// The player clicked Check out a moment ago and is watching
			// the window; the status strip already said so.
			continue
		}
		name := "world #" + itoa(l.WorldID)
		if w := worldFor(now, l.WorldID); w != nil {
			name = w.World.Name
		}
		u.notify("The world is yours", name+" is checked out to this machine — the save is in place.")
	}
}

// holdNearlyUp warns before a hold this machine owns lapses. A lapsed
// hold is somebody else's takeover, and the player may well be in the
// game with the window hidden.
func (u *ui) holdNearlyUp(now companion.State) {
	for _, l := range now.Links {
		if l.SessionID == 0 {
			continue
		}
		w := worldFor(now, l.WorldID)
		if w == nil || w.Holder == nil || w.Holder.SessionID != l.SessionID {
			continue
		}
		left := time.Until(w.Holder.ExpiresAt)
		if left <= 0 || left > holdWarnWithin {
			continue
		}
		u.mu.Lock()
		last := u.holdWarned[l.WorldID]
		fresh := time.Since(last) > holdWarnAgain
		if fresh {
			u.holdWarned[l.WorldID] = time.Now()
		}
		u.mu.Unlock()
		if !fresh {
			continue
		}
		u.notify("Your hold is nearly up",
			w.World.Name+" — renew it, or check it in before someone takes it over.")
	}
}

// syncFailedUnseen reports a *new* failure the player cannot see. With
// the window up the header's error band says it better than a
// notification could, so this only fires from the tray.
func (u *ui) syncFailedUnseen(now companion.State) {
	u.mu.Lock()
	was := u.lastError
	u.lastError = now.Sync.LastError
	u.mu.Unlock()

	if now.Sync.LastError == "" || now.Sync.LastError == was {
		return
	}
	if u.visible.Load() {
		return
	}
	u.notify("Sync failed", now.Sync.LastError)
}

// --- the setting's storage ---

// notifyPrefPath keeps the notification switch beside the window state,
// in this build's own file. The shared companion config is frozen
// surface the browser build reads; it must not grow keys that mean
// nothing to it.
func (u *ui) notifyPrefPath() string {
	return filepath.Join(filepath.Dir(u.cfgPath), "companion-desktop-notify.off")
}

func (u *ui) loadNotifyPref() {
	_, err := os.Stat(u.notifyPrefPath())
	notifyOff.Store(err == nil)
}

func (u *ui) saveNotifyPref(off bool) {
	if off {
		_ = os.WriteFile(u.notifyPrefPath(), []byte("notifications off\n"), 0o600)
		return
	}
	_ = os.Remove(u.notifyPrefPath())
}
