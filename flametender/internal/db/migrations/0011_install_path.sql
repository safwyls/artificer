-- SteamCMD cache repair: optional path to the Palworld install root (the
-- directory holding steamapps/ and steam/), bind-mounted READ-WRITE so
-- Palcon can wipe corrupted manifests and package downloads after a game
-- update breaks the container's updater. Kept separate from save_path and
-- config_path: those mounts stay as narrow as they are, and this one only
-- ever has the contents of its two cache subdirectories deleted. Empty
-- means the repair tool is off for this server.
ALTER TABLE servers ADD COLUMN install_path TEXT NOT NULL DEFAULT '';
