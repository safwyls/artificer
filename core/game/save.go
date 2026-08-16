package game

// SaveLayout is one game's on-disk save shape, consumed by the backup
// archiver (and anything else that must pick "the world" out of a save
// directory). Every field is optional; the zero value is the permissive
// default the archiver documents:
//
//   - the world is the newest non-sidecar regular file (no extension
//     assumed — that assumption is exactly what once made every
//     Enshrouded snapshot silently empty),
//   - `-index`/`-info` suffixed companions are sidecars (a common
//     convention): archived, but never proof a world exists,
//   - every regular file is archived,
//   - verification is a 16-byte size floor.
//
// A game with stricter knowledge overrides: Palworld's layout names
// `Level.sav`, archives only `*.sav`, and verifies the PlZ/PlM/CNK
// container magic — the only mid-write guard a non-atomic writer gets
// (drift ledger, seam 1).
type SaveLayout struct {
	// WorldFile picks the file proving there is a world worth backing up,
	// given the save directory. Nil means newest non-sidecar file.
	WorldFile func(saveDir string) (string, error)
	// IsSidecar marks companion files that are archived but are not
	// worlds. Nil means the `-index`/`-info` suffix convention.
	IsSidecar func(name string) bool
	// IncludeInArchive filters archive membership by save-dir-relative
	// path. Nil means every regular file.
	IncludeInArchive func(rel string) bool
	// VerifyWorld rejects an obviously-broken world file before it is
	// archived. Nil means the size floor.
	VerifyWorld func(path string) error
}
