sqlite3 /root/apps/lojf/nextgen.db <<'SQL'
UPDATE parents
SET phone = REPLACE(REPLACE(REPLACE(REPLACE(REPLACE(phone,' ',''),'-',''),'(',''),')',''), CHAR(13)||CHAR(10), '');

UPDATE parents
SET phone = '+' || SUBSTR(phone, 3)
WHERE phone LIKE '00%';

UPDATE parents
SET phone = '+' || phone
WHERE phone LIKE '62%' AND phone NOT LIKE '+%';

UPDATE parents
SET phone = '+62' || SUBSTR(phone, 2)
WHERE phone LIKE '0%';

UPDATE parents
SET phone = '+' || phone
WHERE phone NOT LIKE '+%';

CREATE UNIQUE INDEX IF NOT EXISTS uniq_parent_phone ON parents(phone);
SQL
