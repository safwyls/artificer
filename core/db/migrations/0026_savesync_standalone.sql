-- Save sync moves to its own service (reliquary) and worlds become
-- fully game-generic (docs/save-sync-architecture.md, the option-B
-- pivot): a world now carries its own game metadata — reported by the
-- companion app that discovered the game — and its own agent link for
-- the server-as-holder flows, instead of leaning on a console's server
-- row. sync_worlds.server_id stays in the schema (migrations are
-- append-only) but nothing reads it any more.

ALTER TABLE sync_worlds ADD COLUMN game_title TEXT NOT NULL DEFAULT '';
-- Where the companion found the save on the reporting player's machine —
-- a hint for the next player's setup, not an instruction anything
-- follows blindly.
ALTER TABLE sync_worlds ADD COLUMN save_hint TEXT NOT NULL DEFAULT '';
-- Free-form game metadata from the companion (Steam app id, install
-- name, …), JSON, shaped by the reporter. The server stores and shows
-- it; it never interprets it.
ALTER TABLE sync_worlds ADD COLUMN game_meta TEXT NOT NULL DEFAULT '';
-- The sidecar agent that can also hold this world (give/take), when the
-- game has a dedicated server. The token is a credential: encrypted at
-- rest like the server rows' agent tokens.
ALTER TABLE sync_worlds ADD COLUMN agent_url TEXT NOT NULL DEFAULT '';
ALTER TABLE sync_worlds ADD COLUMN agent_token_enc TEXT NOT NULL DEFAULT '';
