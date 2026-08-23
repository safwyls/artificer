package main

// The window's own state and lifecycle: what it is showing, how it hears
// that the engine changed, and the tray it lives behind.
//
// The rule this file exists to hold: the UI keeps *no* custody state of
// its own. `u.state` is a companion.Snapshot and nothing else — a value
// re-read from the engine whenever the engine says so. Everything on
// screen is a function of that value, which is why a rebuild is always
// safe and why there is no second copy of the truth to go stale.

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/safwyls/artificer/companion"
)

type ui struct {
	app    fyne.App
	win    fyne.Window
	engine *companion.App

	cfgPath string

	// visible tracks whether the window is on screen rather than hidden
	// in the tray. It gates the presence heartbeat (a hidden window must
	// not keep the custody poll running hot) and decides whether a sync
	// error is worth an OS notification.
	visible atomic.Bool

	mu sync.Mutex
	// state is the engine's snapshot, and the only model the screens
	// read. Never edited here: the UI asks the engine to change
	// something and re-reads what came of it.
	state companion.State
	// art and hints are the two asides the engine resolves through the
	// service. They change when the *set* of games changes, never on a
	// custody poll — asking on every poll is what made the web shelf
	// flicker.
	art        map[string]companion.Art
	artError   string
	artAsked   int
	hintsOK    bool
	hintsKnown int
	hintsError string
	// gameSig is the set of games the asides were last resolved for.
	gameSig string

	// covers holds decoded cover resources per game key, forever. The
	// engine caches the bytes; this caches the fyne.Resource around them,
	// so a rebuild reuses the same object and the image is not re-decoded
	// or re-uploaded to the GPU.
	covers map[string]fyne.Resource
	// coverAsked is the fetches currently in flight, so a redraw landing
	// while a cover is still downloading does not start a second
	// download of the same image.
	coverAsked map[string]bool

	// content is the swappable middle of the window.
	content *fyne.Container
	// header and footer redraw on their own clocks, so they are held.
	headerBox *fyne.Container
	footerBox *fyne.Container
	// statusBox is the transient in-window feedback strip above the
	// footer — the desktop's answer to the web UI's toasts.
	statusBox *fyne.Container
	// tipLayer is the hand-positioned layer hover tips are drawn into,
	// stacked over the whole window. See taparea.go for why it is a
	// layer rather than a Fyne popup.
	tipLayer *fyne.Container

	// selfCheckout is set while a checkout this window started is in
	// flight, so its arriving hold is not announced as a surprise claim.
	selfCheckout atomic.Bool
	// holdWarned is when each world's expiry warning last went out, so a
	// hold nearing its end is mentioned once rather than every tick.
	holdWarned map[int64]time.Time
	// lastError is the previous poll's error, so a *new* failure can be
	// told from a standing one.
	lastError string
	// openDialog is whichever modal is up, so a second request replaces
	// it rather than stacking two.
	openDialog interface{ Hide() }

	// The view's own small decisions — what the player has chosen to
	// look at rather than anything the engine knows. They live here, not
	// in the widgets, because a redraw throws the widgets away.

	// tab is which page the window is showing. Derived states — first
	// run and offline — are *not* tabs: they are computed from the
	// snapshot, so this only ever holds a page the player chose.
	tab tab
	// showHidden is the put-away library entries being shown.
	showHidden bool
	// showOfflineWorlds is the player overriding the offline state's
	// decision to hide the worlds this machine does not hold.
	showOfflineWorlds bool
	// gamesSearch and gamesFilter are the Games tab's own controls.
	// They live here rather than in the widgets because a redraw throws
	// the widgets away, and a search box that empties itself every time
	// the engine polls is unusable.
	gamesSearch string
	gamesFilter gameFilter
	// trailItem is the scan trail's current widget, kept so its open
	// state can be read back before a rebuild discards it; trailChosen
	// is the player's decision once they have made one, and trailSig
	// identifies the scan so a fresh empty one can open itself.
	trailItem   *widget.AccordionItem
	trailChosen *bool
	trailSig    string
}

// snapshot is the current engine state, taken under the UI's lock.
func (u *ui) snapshot() companion.State {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state
}

// refresh re-reads the engine and redraws. Safe from any goroutine: the
// draw is handed to Fyne's own thread.
func (u *ui) refresh() {
	st := u.engine.Snapshot()

	u.mu.Lock()
	prev := u.state
	u.state = st
	u.mu.Unlock()

	u.announce(prev, st)
	u.resolveAsides(st)

	fyne.Do(func() {
		u.redraw()
	})
}

// redraw rebuilds the whole window content from the current snapshot.
//
// Wholesale rather than surgical on purpose: the state is small, the
// covers are cached so nothing re-fetches, and any dialog on screen is
// an overlay that a content rebuild does not touch — the same property
// the web UI relies on when its five-second poll rebuilds the shelf
// under an open form. Must run on Fyne's thread.
func (u *ui) redraw() {
	st := u.snapshot()
	u.headerBox.Objects = []fyne.CanvasObject{u.chrome(st)}
	u.headerBox.Refresh()
	u.footerBox.Objects = []fyne.CanvasObject{u.footer(st)}
	u.footerBox.Refresh()

	u.content.Objects = []fyne.CanvasObject{u.body(st)}
	u.content.Refresh()
}

// chrome is the header and, when there is more than one page to be on,
// the tab bar under it.
//
// The tabs are hidden while the companion is unconfigured: there is
// nothing on either page until it can reach a vault, and offering to
// switch between two empty rooms is not navigation.
func (u *ui) chrome(st companion.State) fyne.CanvasObject {
	if !st.Sync.Configured {
		return u.header(st)
	}
	return container.NewVBox(u.header(st), u.tabBar(st))
}

// build assembles the window's fixed frame: a header that says whether
// the vault is reachable, the content, a feedback strip, and the version
// footer.
func (u *ui) build() fyne.CanvasObject {
	u.mu.Lock()
	u.state = u.engine.Snapshot()
	st := u.state
	u.mu.Unlock()

	u.headerBox = container.NewStack(u.chrome(st))
	u.footerBox = container.NewStack(u.footer(st))
	u.statusBox = container.NewStack()
	u.content = container.NewStack(u.body(st))

	go u.resolveAsides(st)

	bottom := container.NewVBox(u.statusBox, rule(), u.footerBox)
	frame := container.NewBorder(
		container.NewVBox(u.headerBox, rule()),
		bottom, nil, nil,
		container.NewVScroll(container.NewPadded(u.content)),
	)

	// The tooltip layer sits over everything, positioned by hand.
	//
	// It is a layer rather than a Fyne popup because a popup installs a
	// canvas-wide input overlay, and that overlay stole the hover from
	// the very tile the tip was explaining — see taparea.go for the
	// flicker that caused. Nothing in this layer is Tappable or
	// Hoverable, so the pointer passes through it untouched.
	u.tipLayer = container.NewWithoutLayout()
	return container.NewStack(frame, u.tipLayer)
}

// showTipAt puts one tooltip on the layer at a window position, keeping
// it inside the window: a tip that hangs off the bottom or the right
// edge is a tip nobody can read.
func (u *ui) showTipAt(content fyne.CanvasObject, at fyne.Position) {
	if u.tipLayer == nil {
		return
	}
	size := content.MinSize()
	win := u.win.Canvas().Size()
	if at.X+size.Width > win.Width {
		at.X = win.Width - size.Width
	}
	if at.X < 0 {
		at.X = 0
	}
	if at.Y+size.Height > win.Height {
		// No room below: flip above the widget the tip belongs to.
		at.Y = at.Y - size.Height - tipGap*2
	}
	if at.Y < 0 {
		at.Y = 0
	}
	content.Resize(size)
	content.Move(at)
	u.tipLayer.Objects = []fyne.CanvasObject{content}
	u.tipLayer.Refresh()
}

func (u *ui) clearTip() {
	if u.tipLayer == nil || len(u.tipLayer.Objects) == 0 {
		return
	}
	u.tipLayer.Objects = nil
	u.tipLayer.Refresh()
}

// watchEngine turns the engine's change nudges into redraws, and keeps
// the header's freshness line ticking on its own clock — an age has to
// be recomputed even when nothing changed, because the poll it describes
// may answer with an unchanged timestamp. The web header does exactly
// this with a one-second interval.
func (u *ui) watchEngine() {
	changes, stop := u.engine.Subscribe()
	u.win.SetOnClosed(stop)
	go func() {
		for range changes {
			u.refresh()
		}
	}()
	go func() {
		for range time.Tick(time.Second) {
			st := u.snapshot()
			fyne.Do(func() {
				u.headerBox.Objects = []fyne.CanvasObject{u.chrome(st)}
				u.headerBox.Refresh()
			})
		}
	}()
}

// startPresence tells the engine somebody is looking, for as long as the
// window is on screen.
//
// This is the one piece of parity that is invisible until it is missing:
// the custody poll runs at four seconds while watched and a minute
// otherwise, so without this heartbeat the window shows custody up to a
// minute old — somebody else checks a world in, and the person staring
// at the screen sees nothing happen. It stops when the window hides, so
// a companion parked in the tray goes back to being polite.
func (u *ui) startPresence() {
	go func() {
		for {
			if u.visible.Load() {
				u.engine.MarkSeen()
			}
			time.Sleep(5 * time.Second)
		}
	}()
}

// resolveAsides fetches artwork and save-location hints when the set of
// games has changed, and never otherwise. Both are decoration that
// degrades to nothing; neither is allowed to hold up a render.
func (u *ui) resolveAsides(st companion.State) {
	sig := gameSignature(st)
	u.mu.Lock()
	same := sig == u.gameSig
	u.mu.Unlock()
	if same {
		return
	}
	u.mu.Lock()
	u.gameSig = sig
	u.mu.Unlock()

	art := u.engine.Artwork()
	asked, artErr := u.engine.ArtStatus()
	u.mu.Lock()
	u.art, u.artAsked, u.artError = art, asked, artErr
	u.mu.Unlock()
	fyne.Do(u.redraw)

	if st.Sync.Configured {
		available, known, hintErr := u.engine.SaveHints()
		u.mu.Lock()
		u.hintsOK, u.hintsKnown, u.hintsError = available, known, hintErr
		u.mu.Unlock()
		fyne.Do(u.redraw)
	}
}

// gameSignature identifies the set of games, so the asides are re-asked
// when it changes and not when a custody poll lands.
// Connectedness is part of the signature because artwork resolution is
// a call to the service: while the companion is unconfigured the lookup
// returns nothing at all, and without this the set of games would look
// unchanged after connecting and the covers would never be asked for
// again until the next rescan.
func gameSignature(st companion.State) string {
	out := make([]byte, 0, 64)
	if st.Sync.Configured {
		out = append(out, "connected|"...)
	}
	for _, g := range st.Discovered.Games {
		out = append(out, g.Key...)
		out = append(out, '|')
	}
	return string(out)
}

// artFor resolves a cover for anything that can name a game — a
// discovered game, or a link that remembers which one it came from. A
// link made before app ids were recorded still matches by title, which
// is the same fallback the web UI's artFor does.
func (u *ui) artFor(appID, name string) companion.Art {
	u.mu.Lock()
	defer u.mu.Unlock()
	if a, ok := u.art[gameKey(appID, name)]; ok {
		return a
	}
	if name != "" {
		if a, ok := u.art[gameKey("", name)]; ok {
			return a
		}
	}
	return companion.Art{}
}

// cover hands back a decoded cover resource, fetching it at most once
// per game and never on the UI's thread.
//
// The ordering here is the whole bug the desktop shelf had, and it is
// worth being explicit about. A tile asks for its cover while the window
// is being drawn; the artwork lookup that produces the cover *URL* is a
// round trip to the service that has usually not landed yet. The first
// cut asked the engine anyway, the engine found no URL, and both sides
// wrote that down as "this game has no cover" — permanently. Every tile
// in the app showed its fallback name for the life of the process, which
// is exactly what the screenshots showed.
//
// So a cover is only *asked for* once the artwork answer that names it
// has arrived: no art entry, or an entry with no cover URL, means draw
// the fallback and ask nothing. resolveAsides redraws when the artwork
// lands, and this runs again then with a URL to work from.
//
// Everything that arrives is cached forever, misses included, and the
// cache is keyed per game — so a state poll rebuilding the shelf reuses
// the same fyne.Resource objects and the tiles neither flicker nor
// refetch. That is the web UI's rule, kept.
func (u *ui) cover(appID, name string, then func()) fyne.Resource {
	key := gameKey(appID, name)
	u.mu.Lock()
	res, cached := u.covers[key]
	art, resolved := u.art[key]
	if !resolved && name != "" {
		// A link recorded before app ids existed matches by title, the
		// same fallback artFor makes.
		art, resolved = u.art[gameKey("", name)]
	}
	inflight := u.coverAsked[key]
	u.mu.Unlock()
	if cached {
		return res
	}
	if !resolved || strings.TrimSpace(art.Cover) == "" {
		// Nothing to fetch — either the artwork answer is still on its
		// way, or it came back saying this game has no cover.
		return nil
	}
	if inflight {
		return nil
	}
	u.mu.Lock()
	u.coverAsked[key] = true
	u.mu.Unlock()

	go func() {
		data, err := u.engine.Cover(key)
		if err != nil {
			// Decoration: a shelf without covers is still a shelf.
			log.Printf("cover %s: %v", key, err)
		}
		var res fyne.Resource
		if len(data) > 0 {
			// The name carries an extension because Fyne decides how to
			// decode a resource by looking at it. The bytes are whatever
			// the service served — jpeg or png — and image.Decode sniffs
			// that for itself; what the name has to avoid is looking
			// like ".svg".
			res = fyne.NewStaticResource(key+".img", data)
		}
		u.mu.Lock()
		u.covers[key] = res
		delete(u.coverAsked, key)
		u.mu.Unlock()
		if then != nil {
			fyne.Do(then)
		}
	}()
	return nil
}

// --- window lifecycle ---

// showWindow brings the window back from the tray and starts the
// presence heartbeat again.
func (u *ui) showWindow() {
	fyne.Do(func() {
		u.win.Show()
		u.win.RequestFocus()
		u.visible.Store(true)
		u.engine.MarkSeen()
		u.redraw()
	})
}

// hideToTray puts the window away without stopping the sync. The tray
// icon is what says the companion is still there, which is why the tray
// is set up before the window can be hidden.
func (u *ui) hideToTray() {
	u.saveWindowSize()
	u.visible.Store(false)
	u.win.Hide()
}

// quit ends the process — confirming first while a save is moving. A
// killed transfer is the one unforgivable failure this app has, and
// "Transferring a save — don't quit yet" is already the tray's most
// important line; this makes it more than advice.
func (u *ui) quit() {
	if u.snapshot().Sync.Busy {
		u.showWindow()
		u.confirm(
			"A save is being transferred",
			"Quitting now would cut the transfer off part-way. Wait for it to finish, or quit anyway and check the world's state on the service afterwards.",
			"Quit anyway",
			func() { u.reallyQuit() },
		)
		return
	}
	u.reallyQuit()
}

func (u *ui) reallyQuit() {
	u.saveWindowSize()
	u.app.Quit()
}

// --- window state ---

// windowState is what survives a restart. It lives beside the config
// rather than in it: the browser build reads that file too, and it must
// not learn keys that mean nothing to it.
//
// Size only. Fyne exposes no cross-platform window position, so where
// the window sits is the window manager's business; what it remembers is
// how big the player made it.
type windowState struct {
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

func (u *ui) windowStatePath() string {
	return filepath.Join(filepath.Dir(u.cfgPath), "companion-desktop.json")
}

func (u *ui) restoreWindowSize() fyne.Size {
	// The Wails shell's window, and the size the screens are laid out
	// for.
	def := fyne.NewSize(1120, 780)
	data, err := os.ReadFile(u.windowStatePath())
	if err != nil {
		return def
	}
	var ws windowState
	if json.Unmarshal(data, &ws) != nil {
		return def
	}
	// A stored size smaller than the content can use is a size that
	// reads as a broken window; the floor is the honest answer.
	if ws.Width < 720 || ws.Height < 520 {
		return def
	}
	return fyne.NewSize(ws.Width, ws.Height)
}

func (u *ui) saveWindowSize() {
	if u.win == nil {
		return
	}
	size := u.win.Canvas().Size()
	if size.Width < 1 || size.Height < 1 {
		return
	}
	data, err := json.MarshalIndent(windowState{Width: size.Width, Height: size.Height}, "", "  ")
	if err != nil {
		return
	}
	// Best effort: a window that cannot remember its size is a smaller
	// problem than a startup that fails because of it.
	if err := os.WriteFile(u.windowStatePath(), data, 0o600); err != nil {
		log.Printf("window state: %v", err)
	}
}
