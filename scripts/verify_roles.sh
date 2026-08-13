#!/bin/bash
# End-to-end verification of the admin/checkin role split.
# Runs a real server against a throwaway DB. No production contact.
set -u

REPO=$(git rev-parse --show-toplevel)
WORK=$(mktemp -d)
PORT=8099
BASE=http://127.0.0.1:$PORT
PASS=0; FAIL=0

cleanup() { [ -n "${PID:-}" ] && kill "$PID" 2>/dev/null; }
trap cleanup EXIT

ok()   { echo "  PASS  $1"; PASS=$((PASS+1)); }
bad()  { echo "  FAIL  $1 (got: $2)"; FAIL=$((FAIL+1)); }
check(){ [ "$2" = "$3" ] && ok "$1" || bad "$1" "$2 != $3"; }

ln -s "$REPO/templates" "$WORK/templates"
cd "$WORK" || exit 1

ADDR=:$PORT ADMIN_PASSWORD=adminpass123 SESSION_SECRET=verify-secret \
  "$REPO/bin/lojf-nextgen" >"$WORK/server.log" 2>&1 &
PID=$!

for _ in $(seq 1 40); do
  curl -sf "$BASE/health" >/dev/null 2>&1 && break
  sleep 0.25
done
curl -sf "$BASE/health" >/dev/null || { echo "server never came up"; cat "$WORK/server.log"; exit 1; }

# ---- seed: one class today (FJB), one tomorrow (FJB), one today (FJU) ----
TODAY=$(date +%Y-%m-%d)
TOMO=$(date -v+1d +%Y-%m-%d)
sqlite3 "$WORK/nextgen.db" <<SQL
INSERT INTO parents (id,name,phone,created_at,updated_at) VALUES (1,'P','0811',datetime('now'),datetime('now'));
INSERT INTO children (id,parent_id,name,created_at,updated_at) VALUES
  (1,1,'Anak Hari Ini',datetime('now'),datetime('now')),
  (2,1,'Anak Besok',datetime('now'),datetime('now')),
  (3,1,'Anak FJU',datetime('now'),datetime('now'));
INSERT INTO classes (id,name,date,capacity,created_at,updated_at) VALUES
  (1,'FJB Awesome Kids (Feast Jakarta Barat)','$TODAY 00:00:00+07:00',60,datetime('now'),datetime('now')),
  (2,'FJB Little Stars (Feast Jakarta Barat)','$TOMO 00:00:00+07:00',60,datetime('now'),datetime('now')),
  (3,'FJU Stars Club (Feast Jakarta Utara)','$TODAY 00:00:00+07:00',60,datetime('now'),datetime('now'));
INSERT INTO registrations (id,parent_id,child_id,class_id,status,code,created_at,updated_at) VALUES
  (1,1,1,1,'confirmed','REG-TODAY01',datetime('now'),datetime('now')),
  (2,1,2,2,'confirmed','REG-TOMOR01',datetime('now'),datetime('now')),
  (3,1,3,3,'confirmed','REG-FJU0001',datetime('now'),datetime('now'));
SQL

AJAR="$WORK/admin.jar"; VJAR="$WORK/vol.jar"

echo
echo "=== 1. admin login + create a shared FJB check-in account ==="
curl -s -c "$AJAR" -o /dev/null -X POST "$BASE/admin/login" \
  -d "username=admin&password=adminpass123"
code=$(curl -s -b "$AJAR" -o /dev/null -w '%{http_code}' "$BASE/admin/classes")
check "admin can reach /admin/classes" "$code" "200"

curl -s -b "$AJAR" -c "$AJAR" -o /dev/null -X POST "$BASE/admin/users" \
  -d "username=fjb-checkin&password=volunteer123&role=checkin&campus=FJB"
n=$(sqlite3 "$WORK/nextgen.db" "SELECT COUNT(*) FROM admin_users WHERE username='fjb-checkin' AND role='checkin' AND campus='FJB';")
check "fjb-checkin account created" "$n" "1"

echo
echo "=== 2. volunteer login, then confirm the walls ==="
curl -s -c "$VJAR" -o /dev/null -X POST "$BASE/admin/login" \
  -d "username=fjb-checkin&password=volunteer123"

code=$(curl -s -b "$VJAR" -o /dev/null -w '%{http_code}' "$BASE/station")
check "volunteer CAN reach /station" "$code" "200"

for path in /admin/classes /admin/parents /admin/attendance /admin/families /admin/users /admin/roster; do
  code=$(curl -s -b "$VJAR" -o /dev/null -w '%{http_code}' "$BASE$path")
  check "volunteer BLOCKED from $path" "$code" "403"
done

code=$(curl -s -b "$VJAR" -o /dev/null -w '%{http_code}' -X POST "$BASE/admin/registrations/1/cancel")
check "volunteer BLOCKED from cancel" "$code" "403"
code=$(curl -s -b "$VJAR" -o /dev/null -w '%{http_code}' -X POST "$BASE/admin/registrations/1/delete")
check "volunteer BLOCKED from delete" "$code" "403"

echo
echo "=== 3. the 2026-08-07 incident, replayed ==="
# Registration 2 belongs to a class dated TOMORROW. This is exactly what happened.
curl -s -b "$VJAR" -o /dev/null -X POST "$BASE/admin/registrations/2/checkin" \
  -H "Referer: $BASE/station"
got=$(sqlite3 "$WORK/nextgen.db" "SELECT COALESCE(check_in_at,'NULL') FROM registrations WHERE id=2;")
check "volunteer CANNOT check in tomorrow's class" "$got" "NULL"

# Cross-campus: FJU class, FJB account.
curl -s -b "$VJAR" -o /dev/null -X POST "$BASE/admin/registrations/3/checkin" \
  -H "Referer: $BASE/station"
got=$(sqlite3 "$WORK/nextgen.db" "SELECT COALESCE(check_in_at,'NULL') FROM registrations WHERE id=3;")
check "volunteer CANNOT check in another campus" "$got" "NULL"

echo
echo "=== 4. the happy path still works ==="
curl -s -b "$VJAR" -c "$VJAR" -o /dev/null -X POST "$BASE/station/staff" -d "staff_name=Rina"
curl -s -b "$VJAR" -o /dev/null -X POST "$BASE/admin/registrations/1/checkin" \
  -H "Referer: $BASE/station"
got=$(sqlite3 "$WORK/nextgen.db" "SELECT CASE WHEN check_in_at IS NULL THEN 'NULL' ELSE 'SET' END FROM registrations WHERE id=1;")
check "volunteer CAN check in today's own-campus class" "$got" "SET"
by=$(sqlite3 "$WORK/nextgen.db" "SELECT checked_in_by FROM registrations WHERE id=1;")
check "shift name recorded on the row" "$by" "Rina"

echo
echo "=== 5. admin overrides + audit trail ==="
curl -s -b "$AJAR" -o /dev/null -X POST "$BASE/admin/registrations/2/checkin"
got=$(sqlite3 "$WORK/nextgen.db" "SELECT CASE WHEN check_in_at IS NULL THEN 'NULL' ELSE 'SET' END FROM registrations WHERE id=2;")
check "admin CAN check in a future class (override)" "$got" "SET"

denied=$(sqlite3 "$WORK/nextgen.db" "SELECT COUNT(*) FROM audit_logs WHERE action='registration.checkin.denied';")
check "denied attempts are audited" "$denied" "2"
staff=$(sqlite3 "$WORK/nextgen.db" "SELECT staffed_by FROM audit_logs WHERE action='registration.checkin' AND role='checkin' LIMIT 1;")
check "audit row carries the shift name" "$staff" "Rina"

echo
echo "=== 6. old guessable cookie is dead ==="
code=$(curl -s -o /dev/null -w '%{http_code}' -b "admin_session=ok" -L "$BASE/admin/classes")
body=$(curl -s -b "admin_session=ok" -L "$BASE/admin/classes" | grep -c "Masuk")
check "admin_session=ok now lands on login" "$body" "1"

echo
echo "=== 7. brute force lockout ==="
for i in $(seq 1 9); do
  curl -s -o /dev/null -X POST "$BASE/admin/login" -d "username=admin&password=wrong$i"
done
loc=$(curl -s -o /dev/null -w '%{redirect_url}' -X POST "$BASE/admin/login" -d "username=admin&password=adminpass123")
case "$loc" in *error=locked*) ok "locked out after repeated failures";; *) bad "lockout" "$loc";; esac

echo
echo "==================================="
echo "  PASS: $PASS   FAIL: $FAIL"
echo "==================================="
[ "$FAIL" -eq 0 ]
