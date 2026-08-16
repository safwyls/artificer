-- Server settings editing (#7): optional path to the directory holding the
-- server's PalWorldSettings.ini (typically Pal/Saved/Config/LinuxServer),
-- bind-mounted READ-WRITE so Palcon can edit it. This is a separate mount
-- from save_path — which stays read-only — so the precious save data can
-- never be written even when settings are editable. Empty means the settings
-- editor is not configured for this server.
ALTER TABLE servers ADD COLUMN config_path TEXT NOT NULL DEFAULT '';
