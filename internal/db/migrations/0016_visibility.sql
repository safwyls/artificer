-- Per-server view visibility, and per-player opt-outs within it.
--
-- Both columns store what is HIDDEN rather than what is shown, so the empty
-- default means "everything visible" and no existing server changes behaviour
-- when this migration runs. A feature added later is likewise visible until
-- someone turns it off, which is the safe direction for a nav menu but the
-- deliberate one to re-check when the feature is privacy-sensitive.
ALTER TABLE servers ADD COLUMN hidden_features TEXT NOT NULL DEFAULT '';

-- One row per player who has opted out of something. Absent uid = fully
-- visible, so a busy server stores nothing until an admin hides someone.
--
-- Streams, not views: "pals" covers the Player pals, Paldex and Calculators
-- views, because all three read the same /pals payload and hiding a player
-- from one while serving them to another would be a privacy hole with a
-- checkbox in front of it.
CREATE TABLE player_visibility (
    server_id      INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    player_uid     TEXT NOT NULL,
    hidden_streams TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (server_id, player_uid)
);
