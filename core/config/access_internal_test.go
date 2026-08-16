package config

import "testing"

// The Access helpers do real normalization; drift here quietly breaks the
// audience check or the admin-rescue list.
func TestNormalizeTeamDomain(t *testing.T) {
	for in, want := range map[string]string{
		"myteam":                               "myteam.cloudflareaccess.com",
		"myteam.cloudflareaccess.com":          "myteam.cloudflareaccess.com",
		"https://myteam.cloudflareaccess.com/": "myteam.cloudflareaccess.com",
		"  ":                                   "",
	} {
		if got := normalizeTeamDomain(in); got != want {
			t.Errorf("normalizeTeamDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitEmailsFoldsCase(t *testing.T) {
	got := splitEmails(" Admin@Example.com, second@example.com ,")
	if len(got) != 2 || got[0] != "admin@example.com" || got[1] != "second@example.com" {
		t.Errorf("splitEmails = %v", got)
	}
}
