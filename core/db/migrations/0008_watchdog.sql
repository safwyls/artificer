-- Crash watchdog: when enabled (and power control is configured), Palcon
-- restarts the server's container after an unclean exit. Off by default —
-- reviving containers is only wanted where someone chose it.
ALTER TABLE servers ADD COLUMN watchdog INTEGER NOT NULL DEFAULT 0;
