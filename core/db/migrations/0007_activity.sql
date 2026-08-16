-- Activity history: who was on the server, and who did what to it.
--
-- player_events is the collector's join/leave record, one row per observed
-- transition (30s granularity). Sessions and playtime are derived by
-- pairing a player's join with their next leave at read time.
CREATE TABLE player_events (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    ts        TEXT NOT NULL DEFAULT (datetime('now')),
    -- The platform id (steam/xbox) — stable across renames; name is the
    -- display name as of the event.
    user_id   TEXT NOT NULL,
    name      TEXT NOT NULL,
    event     TEXT NOT NULL CHECK (event IN ('join', 'leave'))
);
CREATE INDEX idx_player_events_server_ts ON player_events(server_id, ts);

-- audit_log records management actions taken through Palcon itself:
-- power, saves, broadcasts, moderation, config and automation changes.
-- Deliberately no FK: deleting a server is itself an audited action, and
-- audit rows must survive what they describe.
CREATE TABLE audit_log (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    ts        TEXT NOT NULL DEFAULT (datetime('now')),
    username  TEXT NOT NULL,
    server_id INTEGER NOT NULL,
    action    TEXT NOT NULL,
    detail    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_audit_log_server_ts ON audit_log(server_id, ts);
