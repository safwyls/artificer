-- Automation: scheduled restarts and Discord notifications.
--
-- A server can have any number of restart schedules (daily at 05:00 plus a
-- weekly deep-restart, say). Days and warning lead times are CSV for the
-- same reason user permissions are: small fixed sets, always read whole.
CREATE TABLE restart_schedules (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id       INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    enabled         INTEGER NOT NULL DEFAULT 1,
    -- Weekdays the restart runs, CSV of 0-6 with Sunday = 0 (Go's
    -- time.Weekday numbering), e.g. '0,1,2,3,4,5,6' for every day.
    days            TEXT NOT NULL,
    -- Local wall-clock time of the restart, 'HH:MM' in Palcon's timezone
    -- (the container's TZ), which is when "5am restart" is actually 5am.
    time_of_day     TEXT NOT NULL,
    -- In-game warning broadcasts this many minutes before the restart,
    -- CSV of descending integers, e.g. '15,5,1'. Empty = no warnings.
    warning_minutes TEXT NOT NULL DEFAULT '15,5,1',
    last_run_at     TEXT
);
CREATE INDEX idx_restart_schedules_server ON restart_schedules(server_id);

-- One Discord webhook per server. The URL is a write-only secret (anyone
-- holding it can post to the channel), so it's encrypted at rest like the
-- RCON/REST passwords and never sent back to the browser.
CREATE TABLE discord_webhooks (
    server_id       INTEGER PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    webhook_url_enc TEXT NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 1,
    -- Per-event toggles: unreachable/back-online, joins/leaves, and
    -- scheduled-restart warnings/notices.
    on_status       INTEGER NOT NULL DEFAULT 1,
    on_players      INTEGER NOT NULL DEFAULT 1,
    on_restarts     INTEGER NOT NULL DEFAULT 1
);
