package game

// ConfigCodec is one game's config reader/writer behind a common wire
// shape. The games' codecs (ini and JSON alike) deliberately share their
// policy — never add or remove keys, type-validate, .bak, atomic swap —
// but not their types; this is the seam that keeps the shared config
// handlers game-blind. A game registers its codec on its Definition; a
// nil codec means the console has no settings editor for that game.
type ConfigCodec struct {
	// Filename names the game's settings file — for user-facing labels
	// and 404s, and for the agent config cache on disk.
	Filename string
	// NotConfigured is the codec's sentinel for "no config path set",
	// matched with errors.Is so the handler can answer with setup
	// guidance instead of an error.
	NotConfigured error
	Read          func(path string) (*ConfigPayload, error)
	Write         func(path string, changes map[string]string) error
	// RotateAdminPassword is nil for games whose admin access isn't a
	// password the settings file controls.
	RotateAdminPassword func(path, newPassword string) error
}

// ConfigPayload is the codecs' common Result JSON shape.
type ConfigPayload struct {
	Settings any    `json:"settings"`
	Path     string `json:"path"`
	Writable bool   `json:"writable"`
}
