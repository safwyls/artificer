package palagent

// What PrepareRuntime concretely does for Palworld: seed
// PalWorldSettings.ini from the game's shipped defaults with the identity
// applied exactly once, enforce the management interfaces every start,
// and find the world dir by its newest Level.sav. The kit's lifecycle
// around these hooks is exercised by the shared agent tests; this suite
// pins the game-shaped behavior itself.

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/safwyls/artificer/core/agent"
)

const defaultsIni = `[/Script/Pal.PalGameWorldSettings]
OptionSettings=(Difficulty=None,ServerName="Default Palworld Server",ServerDescription="",AdminPassword="",RCONEnabled=False,RCONPort=25575,RESTAPIEnabled=False,RESTAPIPort=8212)
`

func runtimeEnv(install string, id agent.RuntimeIdentity) agent.RuntimeEnv {
	return agent.RuntimeEnv{
		InstallDir: install,
		ConfigPath: filepath.Join(install, "Pal", "Saved", "Config", "LinuxServer", "PalWorldSettings.ini"),
		Identity:   id,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestPrepareRuntimeSeedsFromDefaultsAndEnforcesManagement(t *testing.T) {
	install := t.TempDir()
	if err := os.WriteFile(filepath.Join(install, "DefaultPalWorldSettings.ini"), []byte(defaultsIni), 0o644); err != nil {
		t.Fatal(err)
	}
	env := runtimeEnv(install, agent.RuntimeIdentity{
		ServerName:    "Pals of the Round Table",
		ServerDesc:    "be nice",
		AdminPassword: "hunter2-but-longer",
	})
	prepareRuntime(env)

	data, err := os.ReadFile(env.ConfigPath)
	if err != nil {
		t.Fatalf("ini was not seeded: %v", err)
	}
	for _, want := range []string{
		`ServerName="Pals of the Round Table"`,
		`ServerDescription="be nice"`,
		`AdminPassword="hunter2-but-longer"`,
		"RCONEnabled=True",
		"RESTAPIEnabled=True",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("ini missing %s:\n%s", want, data)
		}
	}
}

func TestPrepareRuntimeSeedsIdentityExactlyOnce(t *testing.T) {
	install := t.TempDir()
	if err := os.WriteFile(filepath.Join(install, "DefaultPalWorldSettings.ini"), []byte(defaultsIni), 0o644); err != nil {
		t.Fatal(err)
	}
	env := runtimeEnv(install, agent.RuntimeIdentity{ServerName: "First Name"})
	prepareRuntime(env)

	// The operator renames via the settings editor; the next start must
	// not stomp it back to the provisioned name.
	edited := strings.Replace(mustRead(t, env.ConfigPath), `ServerName="First Name"`, `ServerName="Renamed In Dashboard"`, 1)
	if err := os.WriteFile(env.ConfigPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	prepareRuntime(env)
	if got := mustRead(t, env.ConfigPath); !strings.Contains(got, `ServerName="Renamed In Dashboard"`) {
		t.Errorf("second start stomped the operator's rename:\n%s", got)
	}
}

func TestPrepareRuntimeWithoutAdminPasswordLeavesManagementAlone(t *testing.T) {
	install := t.TempDir()
	if err := os.WriteFile(filepath.Join(install, "DefaultPalWorldSettings.ini"), []byte(defaultsIni), 0o644); err != nil {
		t.Fatal(err)
	}
	prepareRuntime(runtimeEnv(install, agent.RuntimeIdentity{}))
	got := mustRead(t, filepath.Join(install, "Pal", "Saved", "Config", "LinuxServer", "PalWorldSettings.ini"))
	if !strings.Contains(got, "RCONEnabled=False") || !strings.Contains(got, "RESTAPIEnabled=False") {
		t.Errorf("management flags flipped without an admin password:\n%s", got)
	}
}

func TestFindSaveDirPicksTheNewestWorld(t *testing.T) {
	install := t.TempDir()
	if _, err := findSaveDir(install); err == nil {
		t.Fatal("fresh install should have no save dir")
	}
	old := filepath.Join(install, "Pal", "Saved", "SaveGames", "0", "AAAA1111")
	cur := filepath.Join(install, "Pal", "Saved", "SaveGames", "0", "BBBB2222")
	for _, d := range []string{old, cur} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "Level.sav"), []byte("PlZx"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(old, "Level.sav"), past, past); err != nil {
		t.Fatal(err)
	}
	got, err := findSaveDir(install)
	if err != nil {
		t.Fatal(err)
	}
	if got != cur {
		t.Errorf("findSaveDir = %s, want the newer world %s", got, cur)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
