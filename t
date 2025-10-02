sqlite3 /root/apps/lojf/nextgen.db <<'SQL'
-- link_codes: add missing timestamp columns
ALTER TABLE link_codes ADD COLUMN created_at DATETIME;
ALTER TABLE link_codes ADD COLUMN updated_at DATETIME;

-- ensure these exist as well
ALTER TABLE link_codes ADD COLUMN expires_at DATETIME;
ALTER TABLE link_codes ADD COLUMN used_at   DATETIME;

-- keep a unique index on code
CREATE UNIQUE INDEX IF NOT EXISTS uniq_link_codes_code ON link_codes(code);

-- telegram_users: add timestamps to match your model
ALTER TABLE telegram_users ADD COLUMN created_at DATETIME;
ALTER TABLE telegram_users ADD COLUMN updated_at DATETIME;
SQL
