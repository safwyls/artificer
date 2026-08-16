-- App-wide key/value settings — the first configuration that belongs to the
-- process rather than to a server row (the advisor's model API key). Secret
-- values are stored encrypted with the same box as server credentials.
CREATE TABLE app_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
