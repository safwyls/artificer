package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/safwyls/artificer/core/config"
)

const (
	goodJWT = "0123456789abcdef0123456789abcdef" // exactly 32
	goodKey = "0123456789abcdef0123456789abcdef" // exactly 32
)

// setEnv clears every variable Load reads, then applies the given ones, so a
// case can't pass because of something the developer has exported.
func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, k := range []string{
		"HTTP_ADDR", "DATA_DIR", "ADMIN_USERNAME", "ADMIN_PASSWORD", "DOCKER_HOST",
		"ANVIL_URL", "ANVIL_TOKEN", "COOKIE_SECURE", "JWT_SECRET", "ENCRYPTION_KEY",
		"ILMARI_URL", "ILMARI_TOKEN", "PROVISIONER_URL", "PROVISIONER_TOKEN",
	} {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	setEnv(t, map[string]string{
		"JWT_SECRET": goodJWT, "ENCRYPTION_KEY": goodKey, "DATA_DIR": dir,
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want the :8080 default", cfg.HTTPAddr)
	}
	if cfg.AdminUsername != "admin" {
		t.Errorf("AdminUsername = %q, want the admin default", cfg.AdminUsername)
	}
	if cfg.CookieSecure {
		t.Error("CookieSecure should default off — flametender is usually plain HTTP on a LAN")
	}
	if cfg.DBPath() != filepath.Join(dir, "flametender.db") {
		t.Errorf("DBPath = %q", cfg.DBPath())
	}
}

func TestLoadReadsTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	setEnv(t, map[string]string{
		"JWT_SECRET": goodJWT, "ENCRYPTION_KEY": goodKey, "DATA_DIR": dir,
		"HTTP_ADDR": ":9999", "ADMIN_USERNAME": "root", "ADMIN_PASSWORD": "hunter2",
		"DOCKER_HOST": "tcp://10.0.0.5:2375",
		"ANVIL_URL":   "http://anvil:8820", "ANVIL_TOKEN": "tok",
		"ANTHROPIC_API_KEY": "sk-ant-test", "GEMINI_API_KEY": "gm-test",
	})

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":9999" || cfg.AdminUsername != "root" || cfg.AdminPassword != "hunter2" {
		t.Errorf("config = %+v", cfg)
	}
	if cfg.DockerHost != "tcp://10.0.0.5:2375" {
		t.Errorf("DockerHost = %q", cfg.DockerHost)
	}
	if cfg.AnvilURL != "http://anvil:8820" || cfg.AnvilToken != "tok" {
		t.Errorf("anvil = %q / %q", cfg.AnvilURL, cfg.AnvilToken)
	}
	if cfg.AnthropicAPIKey != "sk-ant-test" || cfg.GeminiAPIKey != "gm-test" {
		t.Errorf("advisor keys = %q / %q", cfg.AnthropicAPIKey, cfg.GeminiAPIKey)
	}
}

func TestCookieSecureAcceptsBothSpellings(t *testing.T) {
	for _, v := range []string{"true", "1"} {
		setEnv(t, map[string]string{
			"JWT_SECRET": goodJWT, "ENCRYPTION_KEY": goodKey,
			"DATA_DIR": t.TempDir(), "COOKIE_SECURE": v,
		})
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("Load with COOKIE_SECURE=%s: %v", v, err)
		}
		if !cfg.CookieSecure {
			t.Errorf("COOKIE_SECURE=%s did not enable secure cookies", v)
		}
	}

	setEnv(t, map[string]string{
		"JWT_SECRET": goodJWT, "ENCRYPTION_KEY": goodKey,
		"DATA_DIR": t.TempDir(), "COOKIE_SECURE": "yes",
	})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CookieSecure {
		t.Error("only 'true' and '1' should enable secure cookies")
	}
}

// A short JWT secret makes session forgery brute-forceable, and a
// wrong-length encryption key would make stored credentials unrecoverable
// later — both must fail at startup, loudly, rather than at first use.
func TestLoadRejectsWeakSecrets(t *testing.T) {
	cases := []struct {
		name, jwt, key, wantFragment string
	}{
		{"no jwt", "", goodKey, "JWT_SECRET is required"},
		{"short jwt", strings.Repeat("a", 31), goodKey, "at least 32 characters"},
		{"no key", goodJWT, "", "exactly 32 bytes"},
		{"short key", goodJWT, strings.Repeat("a", 31), "exactly 32 bytes"},
		{"long key", goodJWT, strings.Repeat("a", 33), "exactly 32 bytes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, map[string]string{
				"JWT_SECRET": tc.jwt, "ENCRYPTION_KEY": tc.key, "DATA_DIR": t.TempDir(),
			})
			_, err := config.Load()
			if err == nil {
				t.Fatal("Load accepted a weak secret")
			}
			if !strings.Contains(err.Error(), tc.wantFragment) {
				t.Errorf("error %q should mention %q", err, tc.wantFragment)
			}
		})
	}
}

func TestLoadReportsAnUncreatableDataDir(t *testing.T) {
	// A file where the directory should be — mkdir can't win.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := writeFile(file); err != nil {
		t.Fatal(err)
	}
	setEnv(t, map[string]string{
		"JWT_SECRET": goodJWT, "ENCRYPTION_KEY": goodKey,
		"DATA_DIR": filepath.Join(file, "data"),
	})

	if _, err := config.Load(); err == nil {
		t.Fatal("Load accepted a data dir it cannot create")
	} else if !strings.Contains(err.Error(), "creating data dir") {
		t.Errorf("unhelpful error: %v", err)
	}
}

func writeFile(path string) error {
	return os.WriteFile(path, []byte("x"), 0o644)
}

// A deployment carrying a retired name (ILMARI_URL, or the retired
// provisioner-mode pair) loses the feature silently otherwise: the console
// simply boots without a provisioner and the wizard is absent, with nothing
// pointing at the setting that stopped being read.
func TestRetiredSettingsReportsOnlyUnreplacedNames(t *testing.T) {
	setEnv(t, map[string]string{"ILMARI_URL": "http://ilmari:8820"})
	got := config.RetiredSettings()
	if len(got) != 1 || !strings.Contains(got[0], "ILMARI_URL") || !strings.Contains(got[0], "ANVIL_URL") {
		t.Fatalf("RetiredSettings() = %q, want one message naming ILMARI_URL and ANVIL_URL", got)
	}

	// Already migrated: the old name may linger in a compose file, but the
	// replacement is set, so there is nothing to warn about.
	setEnv(t, map[string]string{
		"ILMARI_URL": "http://ilmari:8820", "ANVIL_URL": "http://anvil:8820",
	})
	if got := config.RetiredSettings(); len(got) != 0 {
		t.Errorf("RetiredSettings() = %q, want silence once ANVIL_URL is set", got)
	}

	// Both retired pairs point at the same replacements, and each old name
	// is reported on its own.
	setEnv(t, map[string]string{
		"ILMARI_TOKEN": "a", "PROVISIONER_URL": "http://legacy-agent:8811",
	})
	if got := config.RetiredSettings(); len(got) != 2 {
		t.Errorf("RetiredSettings() = %q, want one message per retired name", got)
	}

	setEnv(t, nil)
	if got := config.RetiredSettings(); len(got) != 0 {
		t.Errorf("RetiredSettings() = %q on a clean environment", got)
	}
}
