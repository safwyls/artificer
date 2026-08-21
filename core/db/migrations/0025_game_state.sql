-- Small per-server state a game module needs to outlive the process.
--
-- Core owns the table and knows nothing about what goes in it: a game
-- namespaces its own rows with `scope` and shapes `value` itself (JSON by
-- convention). This is the durable counterpart to the in-memory state
-- games already keep — the first user is Dragonwilds' character-sheet
-- memory, which has to survive a console restart to mean anything.
--
-- Deliberately not a general cache: rows are few, small, and each one is
-- something the console observed and would otherwise have no way to
-- re-observe. Anything reconstructible from a live source belongs in
-- memory instead.
CREATE TABLE game_state (
    server_id  INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    -- The game module's namespace for these rows.
    scope      TEXT NOT NULL,
    -- Identity within the scope (a character guid, for the first user).
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    -- When the stored fact was true — supplied by the writer, not the
    -- clock: a character sheet is as old as the save it came from.
    updated_at TEXT NOT NULL,
    PRIMARY KEY (server_id, scope, key)
);
