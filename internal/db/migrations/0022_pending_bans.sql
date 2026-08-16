-- Ban edits the console made that the game has not applied yet.
--
-- `bannedAccounts` in enshrouded_server.json has two writers: this console
-- and the running game, whose own kick/ban UI maintains the same list. The
-- game holds it in memory while it is up and writes it out on shutdown, so
-- a console edit made mid-session is reverted by the game's own copy at the
-- next stop — which is exactly what a real deployment showed on 2026-08-16.
--
-- So a ban made while the server is running is stored here as intent, and
-- applied to the file immediately before the next start, in the one window
-- where nothing else owns it. Rows are cleared once applied: the intent is
-- "make this change", not "hold this state" — the file remains the ban
-- list, and a later in-game ban of the same account must not be undone by
-- a stale lift sitting in this table.
-- A row survives until the *file* agrees with it, not until it is written.
-- That is what makes this self-diagnosing: applied_at records that the
-- console wrote the change into a stopped server's config, so a row still
-- unsatisfied after that is proof the game overwrote it — the one thing
-- the console could not tell the operator before.
CREATE TABLE pending_bans (
    server_id  INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    account_id TEXT NOT NULL,
    -- 'ban' adds the account to the file, 'lift' removes it.
    action     TEXT NOT NULL CHECK (action IN ('ban', 'lift')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    -- Set when the change was written into the config with the game not
    -- running. NULL while the edit is merely queued.
    applied_at TEXT,
    PRIMARY KEY (server_id, account_id)
);
