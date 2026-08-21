-- Save sync: checkout/check-in custody of a shared world save, backed by
-- versioned archives on the console's data volume
-- (docs/save-sync-architecture.md). Three tables and one user column.
--
-- The lock is deliberately not a table: it is the unique ACTIVE session
-- row, enforced by the partial index below, so there is no second
-- representation to drift from the sessions themselves.

CREATE TABLE sync_worlds (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    -- Optional dedicated-server link: the server that can also hold this
    -- world (the checkout-from-server / check-in-to-server flows).
    server_id     INTEGER REFERENCES servers(id) ON DELETE SET NULL,
    lease_hours   INTEGER NOT NULL DEFAULT 48,
    max_bytes     INTEGER NOT NULL DEFAULT 268435456, -- 256 MiB
    keep_versions INTEGER NOT NULL DEFAULT 20,
    checkpoints   INTEGER NOT NULL DEFAULT 1,
    -- Discord webhook for this world's events (checkout, check-in, expiry,
    -- conflict). Worlds are not servers, so they don't ride the per-server
    -- webhook rows; a friend group's world usually has its own channel.
    webhook_url   TEXT NOT NULL DEFAULT '',
    -- The canonical current version. No foreign key: versions reference
    -- sessions reference worlds, and sqlite cannot express the cycle; the
    -- store layer maintains it.
    head_version  INTEGER,
    -- Claim-next: the single queued next holder, if any.
    next_holder   INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at    TEXT NOT NULL
);

CREATE TABLE sync_sessions (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    world_id       INTEGER NOT NULL REFERENCES sync_worlds(id) ON DELETE CASCADE,
    holder_id      INTEGER NOT NULL REFERENCES users(id),
    -- 1 = the linked dedicated server holds the world; holder_id is then
    -- the admin who drove the move.
    server_held    INTEGER NOT NULL DEFAULT 0,
    -- The version delivered at checkout; NULL for a world with no
    -- versions yet.
    base_version   INTEGER,
    -- active | returned | reclaimed | released. "Expired" is not a
    -- status: it is an active session past expires_at, which changes
    -- what OTHERS may do (claim it), not what the holder may do.
    status         TEXT NOT NULL DEFAULT 'active',
    checked_out_at TEXT NOT NULL,
    expires_at     TEXT NOT NULL,
    -- When the expiry warning went out, so it goes out once per lease.
    warned_at      TEXT,
    ended_at       TEXT,
    ended_by       INTEGER REFERENCES users(id)
);

-- The lock. One active session per world, enforced by the database
-- rather than by anything in memory.
CREATE UNIQUE INDEX sync_sessions_one_active ON sync_sessions(world_id) WHERE status = 'active';

CREATE TABLE sync_versions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    world_id    INTEGER NOT NULL REFERENCES sync_worlds(id) ON DELETE CASCADE,
    session_id  INTEGER REFERENCES sync_sessions(id),
    -- Lineage. SET NULL on delete: retention prunes parents out from
    -- under kept children, and a kept version with a pruned parent reads
    -- as exactly that rather than blocking retention forever.
    parent_id   INTEGER REFERENCES sync_versions(id) ON DELETE SET NULL,
    kind        TEXT NOT NULL, -- checkin | checkpoint | import
    -- A check-in whose session could no longer move the head. Kept and
    -- flagged, exempt from pruning, until a human picks a head.
    conflict    INTEGER NOT NULL DEFAULT 0,
    bytes       INTEGER NOT NULL,
    sha256      TEXT NOT NULL,
    uploader_id INTEGER REFERENCES users(id),
    created_at  TEXT NOT NULL
);

CREATE INDEX sync_versions_world ON sync_versions(world_id);

-- The companion app's per-player credential; empty = none minted.
-- Per player rather than per server (unlike companion_token): the token
-- names WHO holds a save, and revoking one person must not revoke the
-- whole group.
ALTER TABLE users ADD COLUMN sync_token TEXT NOT NULL DEFAULT '';
