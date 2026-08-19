-- Companion token: lets a player-side companion app push data to this
-- server's console without a login (mirrors public_token: empty = off,
-- token-in-URL = the credential). Write-scoped and game-interpreted —
-- the routes behind it are contributed by the game module.
ALTER TABLE servers ADD COLUMN companion_token TEXT NOT NULL DEFAULT '';
