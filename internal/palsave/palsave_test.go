package palsave

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func havePython(t *testing.T, module string) bool {
	t.Helper()
	return exec.Command("python3", "-c", "import "+module).Run() == nil
}

// assertFixture checks the extraction of the shared two-player fixture. Both
// save containers hold identical data, so both must produce identical output.
func assertFixture(t *testing.T, result *Result) {
	t.Helper()

	if len(result.Players) != 2 {
		t.Fatalf("want 2 players, got %d", len(result.Players))
	}
	kyoshi := result.Players[0]
	if kyoshi.Nickname != "Kyoshi" || kyoshi.Level != 42 {
		t.Fatalf("unexpected first player: %+v", kyoshi)
	}
	if len(kyoshi.Party) != 2 || len(kyoshi.Palbox) != 2 || len(kyoshi.Base) != 1 {
		t.Fatalf("kyoshi buckets wrong: party=%d palbox=%d base=%d",
			len(kyoshi.Party), len(kyoshi.Palbox), len(kyoshi.Base))
	}
	boss := kyoshi.Party[1]
	if boss.CharacterID != "BOSS_Anubis" || !boss.IsBoss || boss.TalentHP != 100 {
		t.Fatalf("unexpected boss pal: %+v", boss)
	}
	if !kyoshi.Palbox[1].IsLucky {
		t.Fatalf("Kitsunebi should be lucky: %+v", kyoshi.Palbox[1])
	}

	ren := result.Players[1]
	if ren.Nickname != "Ren" || len(ren.Party) != 1 || len(ren.Palbox) != 1 || len(ren.Base) != 0 {
		t.Fatalf("unexpected ren: %+v", ren)
	}
}

// A cached result for one save must return immediately even while another
// save's parse holds the parse lock — with several servers configured, the
// pals/map pages of server A shouldn't stall on server B's slow parse.
func TestCachedReadNotBlockedByOtherParse(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.py")
	script := "import json, time\ntime.sleep(1)\nprint(json.dumps({\"players\": [], \"guilds\": []}))\n"
	if err := os.WriteFile(stub, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := &Reader{scriptPath: stub, cache: make(map[string]cacheEntry)}

	makeSave := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "Level.sav"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	saveA, saveB := makeSave("a"), makeSave("b")

	// Prime the cache for A (pays one stub parse).
	if _, err := reader.Read(context.Background(), saveA); err != nil {
		t.Fatal(err)
	}

	// B's parse runs in the background, holding the parse lock ~1s.
	done := make(chan error, 1)
	go func() {
		_, err := reader.Read(context.Background(), saveB)
		done <- err
	}()
	time.Sleep(100 * time.Millisecond) // let B reach the extractor

	begin := time.Now()
	if _, err := reader.Read(context.Background(), saveA); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(begin); d > 500*time.Millisecond {
		t.Errorf("cached read for A blocked %v behind B's parse", d)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// A stale entry must be served immediately — not block behind the re-parse —
// and the re-parse must land in the cache shortly after.
func TestReadServeStale(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.py")
	script := "import json, time\ntime.sleep(1)\nprint(json.dumps({\"players\": [], \"guilds\": []}))\n"
	if err := os.WriteFile(stub, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := &Reader{scriptPath: stub, cache: make(map[string]cacheEntry)}

	save := filepath.Join(dir, "world")
	if err := os.MkdirAll(save, 0o755); err != nil {
		t.Fatal(err)
	}
	sav := filepath.Join(save, "Level.sav")
	if err := os.WriteFile(sav, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// First load has nothing to serve, so it blocks on the parse.
	first, err := reader.ReadServeStale(context.Background(), save)
	if err != nil {
		t.Fatal(err)
	}

	// The save moves on; the stale parse must come back without waiting the
	// stub's full second.
	if err := os.Chtimes(sav, time.Now(), time.Now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	begin := time.Now()
	stale, err := reader.ReadServeStale(context.Background(), save)
	if err != nil {
		t.Fatal(err)
	}
	if stale != first {
		t.Fatal("expected the stale cached result while refresh runs")
	}
	if d := time.Since(begin); d > 500*time.Millisecond {
		t.Errorf("stale serve blocked %v behind the refresh", d)
	}

	// The background refresh replaces the entry.
	deadline := time.Now().Add(5 * time.Second)
	for {
		fresh, err := reader.ReadServeStale(context.Background(), save)
		if err != nil {
			t.Fatal(err)
		}
		if fresh != first {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background refresh never landed")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Refresh parses only when there is real work: a changed, settled save.
func TestRefresh(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub.py")
	script := "import json\nprint(json.dumps({\"players\": [], \"guilds\": []}))\n"
	if err := os.WriteFile(stub, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	reader := &Reader{scriptPath: stub, cache: make(map[string]cacheEntry)}

	save := filepath.Join(dir, "world")
	if err := os.MkdirAll(save, 0o755); err != nil {
		t.Fatal(err)
	}
	sav := filepath.Join(save, "Level.sav")
	if err := os.WriteFile(sav, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	settle := func(age time.Duration) {
		if err := os.Chtimes(sav, time.Now(), time.Now().Add(-age)); err != nil {
			t.Fatal(err)
		}
	}

	settle(time.Minute)
	if parsed, err := reader.Refresh(context.Background(), save); err != nil || !parsed {
		t.Fatalf("cold refresh: parsed=%v err=%v, want a parse", parsed, err)
	}
	if parsed, err := reader.Refresh(context.Background(), save); err != nil || parsed {
		t.Fatalf("fresh refresh: parsed=%v err=%v, want a no-op", parsed, err)
	}

	// Just-written saves are left alone until they settle.
	if err := os.Chtimes(sav, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if parsed, err := reader.Refresh(context.Background(), save); err != nil || parsed {
		t.Fatalf("unsettled refresh: parsed=%v err=%v, want a no-op", parsed, err)
	}

	// Once settled, the change is picked up.
	settle(10 * time.Second)
	if parsed, err := reader.Refresh(context.Background(), save); err != nil || !parsed {
		t.Fatalf("settled refresh: parsed=%v err=%v, want a parse", parsed, err)
	}
}

// assertInventory checks the item containers the newlayout fixture carries.
// Slot data is packed into each slot's RawData, and gear/egg state lives in a
// separate section joined by a dynamic-item guid, so a silent misread here
// would surface as plausible-looking wrong numbers rather than an error.
func assertInventory(t *testing.T, kyoshi, ren PlayerPals) {
	t.Helper()

	bag, ok := kyoshi.Inventory["common"]
	if !ok {
		t.Fatalf("kyoshi has no backpack: %+v", kyoshi.Inventory)
	}
	if bag.Size != 6 {
		t.Fatalf("backpack size = %d, want 6", bag.Size)
	}
	// Three occupied slots out of five written: the zero-count one is empty
	// and dropped, and slot 2 was never written at all. Slot numbers are the
	// point — the viewer draws the bag by them.
	want := []ItemSlot{
		{Slot: 0, ItemID: "Money", Count: 1200},
		{Slot: 1, ItemID: "PalSphere_Mega", Count: 42},
		{Slot: 4, ItemID: "PalEgg_Fire_01", Count: 1, EggSpecies: "Kitsunebi"},
	}
	if !reflect.DeepEqual(bag.Slots, want) {
		t.Fatalf("kyoshi backpack = %+v, want %+v", bag.Slots, want)
	}

	// Gear state comes from DynamicItemSaveData, keyed by the guid on the slot.
	arms := kyoshi.Inventory["weapons"]
	if len(arms.Slots) != 1 || arms.Slots[0].Durability != 2857 || arms.Slots[0].Ammo != 1 {
		t.Fatalf("kyoshi weapons = %+v, want one bow at 2857 durability with 1 round", arms.Slots)
	}
	if keys := kyoshi.Inventory["essential"]; keys.Size != 230 || len(keys.Slots) != 1 {
		t.Fatalf("kyoshi key items = %+v, want 1 of 230 slots", keys)
	}

	// A weapon that rolled its own passive, on a player whose save declares
	// only a backpack — the other container fields are absent entirely.
	sword := ren.Inventory["common"]
	if len(sword.Slots) != 1 || !reflect.DeepEqual(sword.Slots[0].Passives, []string{"Legend"}) {
		t.Fatalf("ren backpack = %+v, want one katana with a Legend passive", sword.Slots)
	}
	for _, role := range []string{"essential", "weapons", "equipment"} {
		if _, ok := ren.Inventory[role]; ok {
			t.Fatalf("ren should have no %q container: %+v", role, ren.Inventory)
		}
	}

	assertCharacter(t, kyoshi, ren)

	// The fixture's world chest belongs to nobody and must not be attributed.
	for _, p := range []PlayerPals{kyoshi, ren} {
		for role, c := range p.Inventory {
			for _, s := range c.Slots {
				if s.ItemID == "Wood" {
					t.Fatalf("%s picked up the unowned chest's contents in %q", p.Nickname, role)
				}
			}
		}
	}
}

// assertCharacter checks the player's own save entry: level progress,
// condition and stat-point spend. The point lists arrive with Japanese stat
// names whatever language the server runs in, so a missed mapping would show
// up as an untranslated label rather than an error.
func assertCharacter(t *testing.T, kyoshi, ren PlayerPals) {
	t.Helper()

	c := kyoshi.Character
	if c == nil {
		t.Fatal("kyoshi has no character record")
	}
	// Hp and ShieldHP are FixedPoint64 — stored ×1000.
	if c.Exp != 1234567 || c.HP != 6820 || c.Shield != 1045 || c.Stomach != 89.5 {
		t.Fatalf("kyoshi condition = %+v, want exp 1234567, hp 6820, shield 1045, stomach 89.5", c)
	}
	if c.UnusedStatusPoints != 2 {
		t.Fatalf("kyoshi unused points = %d, want 2", c.UnusedStatusPoints)
	}
	// Stats the player put nothing into are dropped, so a build reads as what
	// was actually invested rather than a list padded with zeroes.
	wantPoints := map[string]int{
		"Max HP": 17, "Max SP": 18, "Attack": 18, "Carry Weight": 18,
		"Capture Rate": 7, "Movement Speed": 15,
	}
	if !reflect.DeepEqual(c.StatusPoints, wantPoints) {
		t.Fatalf("kyoshi status points = %v, want %v", c.StatusPoints, wantPoints)
	}
	if !reflect.DeepEqual(c.ExStatusPoints, map[string]int{"Max HP": 9, "Attack": 7}) {
		t.Fatalf("kyoshi ex status points = %v", c.ExStatusPoints)
	}
	if c.FoodBuff != "Minestrone" || c.FoodBuffSeconds != 367 {
		t.Fatalf("kyoshi food buff = %q/%ds, want Minestrone/367s", c.FoodBuff, c.FoodBuffSeconds)
	}

	// Ren's entry carries none of it — a player who has never spent a point
	// must read as an empty build, not a missing record.
	if ren.Character == nil {
		t.Fatal("ren should still have a character record")
	}
	if len(ren.Character.StatusPoints) != 0 || ren.Character.Exp != 0 || ren.Character.FoodBuff != "" {
		t.Fatalf("ren character should be empty: %+v", ren.Character)
	}
}

// The fixtures are synthetic — see testdata/README.md — so they exercise the
// real decompress/GVAS-parse/extract path with no copyrighted game data.
func TestRead(t *testing.T) {
	if !havePython(t, "palworld_save_tools") {
		t.Skip("python3 with palworld-save-tools not available")
	}

	tests := []struct {
		name string
		// path is relative to this package; a directory must resolve to the
		// Level.sav inside it, a file must be read directly. Empty means the
		// fixture is generated into a temp dir by gen_newlayout_fixture.py —
		// it needs only palworld_save_tools, which this test already
		// requires, and generating keeps the storage sidecars covered
		// without committing more .sav binaries.
		path       string
		needsOodle bool
		// The newlayout fixture also carries pal-storage files: a
		// Players/<uid>_dps.sav for Kyoshi (two pals plus an empty slot)
		// and a GlobalPalStorage.sav pal old-owned by Ren.
		hasStorage bool
	}{
		{name: "PlZ/zlib via directory", path: "testdata"},
		{name: "PlM/oodle via file", path: "testdata/Level_oodle.sav", needsOodle: true},
		// 0.6-era layout: pals carry no OwnerPlayerUId and players keep their
		// container ids in Players/<uid>.sav, so ownership resolves by
		// container. Produced zero pals for every player before that was
		// handled — hence a fixture rather than trusting the old one.
		{name: "container-based ownership", hasStorage: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsOodle && !havePython(t, "ooz") {
				t.Skip("python3 with pyooz not available")
			}
			reader, err := NewReader(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			fixture := tc.path
			if fixture == "" {
				dir := t.TempDir()
				gen := exec.Command("python3", "gen_newlayout_fixture.py", dir)
				gen.Dir = "testdata"
				if out, err := gen.CombinedOutput(); err != nil {
					t.Fatalf("generating newlayout fixture: %v: %s", err, out)
				}
				fixture = dir
			}
			path, err := filepath.Abs(fixture)
			if err != nil {
				t.Fatal(err)
			}
			result, err := reader.Read(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			assertFixture(t, result)

			kyoshi, ren := result.Players[0], result.Players[1]
			if tc.hasStorage {
				if len(kyoshi.Storage) != 2 || kyoshi.Storage[0].CharacterID != "Bastet" || kyoshi.Storage[1].TalentShot != 100 {
					t.Fatalf("kyoshi storage wrong: %+v", kyoshi.Storage)
				}
				if len(ren.Storage) != 1 || ren.Storage[0].CharacterID != "Umihebi" {
					t.Fatalf("ren storage wrong: %+v", ren.Storage)
				}
				// A storage pal's slot is its position in the sidecar's array,
				// not its own SlotId (stale in storage) — so the empty slot 1
				// between the fixture's two pals leaves JetDragon at 2, which
				// is what the UI turns into "page 1, slot 3".
				if kyoshi.Storage[0].SlotIndex != 0 || kyoshi.Storage[1].SlotIndex != 2 {
					t.Fatalf("kyoshi storage slots = %d, %d; want 0, 2",
						kyoshi.Storage[0].SlotIndex, kyoshi.Storage[1].SlotIndex)
				}
				if ren.Storage[0].SlotIndex != 0 {
					t.Fatalf("ren storage slot = %d, want 0", ren.Storage[0].SlotIndex)
				}
				// Paldex records ride in Players/<uid>.sav: three registered
				// species (Penguin's flag is false — seen, not registered),
				// and capture counts. Ren's save has no RecordData at all,
				// which must yield empty, not missing, fields.
				if len(kyoshi.Paldeck) != 3 || kyoshi.Captures["SheepBall"] != 4 || len(kyoshi.Captures) != 2 {
					t.Fatalf("kyoshi paldex wrong: deck=%v captures=%v", kyoshi.Paldeck, kyoshi.Captures)
				}
				if len(ren.Paldeck) != 0 || len(ren.Captures) != 0 {
					t.Fatalf("ren paldex should be empty: deck=%v captures=%v", ren.Paldeck, ren.Captures)
				}
				// The base pal ties to its camp via the WorkerDirector's
				// container id; party/palbox pals carry no base.
				if kyoshi.Base[0].BaseID != "eeeeeeee-0000-0000-0000-000000000001" {
					t.Fatalf("base pal BaseID = %q, want the fixture camp id", kyoshi.Base[0].BaseID)
				}
				if kyoshi.Party[0].BaseID != "" {
					t.Fatalf("party pal BaseID = %q, want empty", kyoshi.Party[0].BaseID)
				}
				assertInventory(t, kyoshi, ren)
			} else if len(kyoshi.Storage) != 0 || len(ren.Storage) != 0 {
				t.Fatalf("unexpected storage pals: %+v %+v", kyoshi.Storage, ren.Storage)
			}

			// A second read of an unchanged file must come from cache —
			// verified by pointer identity, since a re-parse would allocate.
			again, err := reader.Read(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if again != result {
				t.Fatal("expected cached result on unchanged mtime")
			}
		})
	}
}
