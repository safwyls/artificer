package palworld

import (
	"context"

	"github.com/safwyls/sampo/core/api"
	"github.com/safwyls/sampo/core/store"
	"github.com/safwyls/sampo/games/palworld/palsave"
)

// Roster is Palworld's save-derived roster for the visibility editor
// (drift ledger seam 6 — the working roster a naive take-F would have
// replaced with a stub).
type Roster struct {
	Reader *palsave.Reader
}

var _ api.RosterSource = (*Roster)(nil)

func (ro *Roster) Roster(ctx context.Context, srv *store.Server, savePath string) ([]api.RosterEntry, error) {
	result, err := ro.Reader.ReadServeStale(ctx, savePath)
	if err != nil {
		return nil, err
	}
	out := make([]api.RosterEntry, 0, len(result.Players))
	for _, p := range result.Players {
		out = append(out, api.RosterEntry{UID: p.UID, Nickname: p.Nickname, Level: p.Level})
	}
	return out, nil
}
