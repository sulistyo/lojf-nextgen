sqlite3 /root/apps/lojf/nextgen.db <<'SQL'
DROP INDEX IF EXISTS uniq_parents_email;
CREATE UNIQUE INDEX IF NOT EXISTS uniq_parents_email
ON parents(email COLLATE NOCASE)
WHERE email <> '';
SQL
