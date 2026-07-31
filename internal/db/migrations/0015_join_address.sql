-- The address players outside the LAN type to connect. `host` is how palcon
-- reaches the server for management, which on a home setup is a private IP
-- (10.x / 192.168.x) — useless to anyone joining from the internet. Empty
-- falls back to host:game_port, which is right for LAN-only servers.
ALTER TABLE servers ADD COLUMN join_address TEXT NOT NULL DEFAULT '';
