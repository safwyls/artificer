package host

import "testing"

// Containers created while this service was called ilmari carry the old
// label namespace. The rename must not orphan them: a live host's managed
// servers have to stay recognised as owned, or destroy/rebuild/adopt would
// treat them as foreign. New containers get the anvil.* labels; the old
// ones are still read.
func TestOwnerOfRecognisesFormerIlmariLabels(t *testing.T) {
	cases := []struct {
		name           string
		labels         map[string]string
		wantOwner      string
		wantSlug       string
		wantRecognised bool
	}{
		{
			name:      "current anvil labels",
			labels:    map[string]string{LabelManaged: "true", LabelOwner: "palcon", LabelSlug: "fluffy"},
			wantOwner: "palcon", wantSlug: "fluffy", wantRecognised: true,
		},
		{
			name:      "former ilmari labels are still ours",
			labels:    map[string]string{"ilmari.managed": "true", "ilmari.owner": "wildskeeper", "ilmari.slug": "ashenfall"},
			wantOwner: "wildskeeper", wantSlug: "ashenfall", wantRecognised: true,
		},
		{
			name:      "pre-service per-console labels still ours",
			labels:    map[string]string{"palcon.provisioned": "true", "palcon.slug": "palhalla"},
			wantOwner: "palcon", wantSlug: "palhalla", wantRecognised: true,
		},
		{
			name:           "unlabelled container is nobody's",
			labels:         map[string]string{"com.docker.compose.project": "something"},
			wantRecognised: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, slug, ok := ownerOf(tc.labels)
			if ok != tc.wantRecognised {
				t.Fatalf("recognised = %v, want %v", ok, tc.wantRecognised)
			}
			if ok && (owner != tc.wantOwner || slug != tc.wantSlug) {
				t.Errorf("owner/slug = %q/%q, want %q/%q", owner, slug, tc.wantOwner, tc.wantSlug)
			}
		})
	}
}
