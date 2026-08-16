-- Automated save backups: zip snapshots of the (read-only) save mount into
-- DATA_DIR/backups/<server id>/. Interval 0 = no schedule (manual backups
-- still work); keep is count-based retention so disk use stays bounded.
ALTER TABLE servers ADD COLUMN backup_interval_hours INTEGER NOT NULL DEFAULT 0;
ALTER TABLE servers ADD COLUMN backup_keep INTEGER NOT NULL DEFAULT 14;
