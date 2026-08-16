-- Per-user advisor keys: a palcon user may bring their own model API key,
-- used in place of the shared one for their requests only. One row per
-- user, encrypted like every other stored credential; the row dies with
-- the account.
CREATE TABLE user_advisor_keys (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
