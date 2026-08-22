-- Asking a holder's companion to hand the world back.
--
-- The companion polls; nothing can reach into it. So a request is a flag
-- on the session that the next poll picks up — the same shape as a
-- queued claim, and the only shape available to a client behind a
-- household router.
--
-- Two kinds. "checkpoint" captures the holder's current save without
-- disturbing their session, for a backup while someone is playing.
-- "checkin" captures it *and* ends the hold, which is the one that
-- answers "the host went to bed mid-session and nobody else can play":
-- a checkpoint alone never moves the head, so it would leave the next
-- player checking out a save from before the session started.
ALTER TABLE sync_sessions ADD COLUMN requested_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE sync_sessions ADD COLUMN requested_at TEXT;
ALTER TABLE sync_sessions ADD COLUMN requested_by INTEGER REFERENCES users(id);
