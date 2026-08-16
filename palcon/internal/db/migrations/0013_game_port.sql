-- The port players join on (UDP). Palcon only ever needed the management
-- ports before; the dashboard now shows a copyable join address, and the
-- provisioning wizard records what it published. 8211 is the game's
-- default.
ALTER TABLE servers ADD COLUMN game_port INTEGER NOT NULL DEFAULT 8211;
