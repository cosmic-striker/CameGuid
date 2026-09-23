#!/usr/bin/env bash
#
# End-to-end smoke test for the hardened CamGuid stack (CI use).
#
# Boots db + redis + api + relay via docker compose (no WAN/RTSP needed —
# we validate the security flow, not media transport) and exercises:
#   1. bootstrap admin login              -> must_change_password=true
#   2. forced password change             -> re-login yields=false
#   3. device creation                   (stream_url stored encrypted)
#   4. stream token mint                  (RBAC-guarded)
#   5. relay verify + DB-resolve start    (valid token -> 200, bogus -> 401)
#   6. audit log presence                 (WriteAudit on login/device.create)
#
set -euo pipefail

cd "$(dirname "$0")/../.."

# --- Compose interpolation vars (every referenced var must exist) ----------
export DB_PASSWORD="${DB_PASSWORD:-camguid_smoke}"
export JWT_SECRET="${JWT_SECRET:-smoke_jwt_$(openssl rand -hex 16)}"
export ENCRYPTION_KEY="${ENCRYPTION_KEY:-$(openssl rand -hex 32)}"
export CAMGUID_STREAM_SIGNING_SECRET="${CAMGUID_STREAM_SIGNING_SECRET:-$(openssl rand -hex 32)}"
export NVR_ENCRYPTION_KEY="${NVR_ENCRYPTION_KEY:-$(openssl rand -base64 44 | tr -d '\n')}"
export NVR_STREAM_SIGNING_SECRET="${NVR_STREAM_SIGNING_SECRET:-$(openssl rand -hex 32)}"
export BOOTSTRAP_ADMIN_PASSWORD="${BOOTSTRAP_ADMIN_PASSWORD:-smoke-admin-$(openssl rand -hex 6)}"
export LOG_LEVEL="${LOG_LEVEL:-warn}"
export LOGIN_RATE_LIMIT=20
export LOGIN_RATE_WINDOW=60

NEW_ADMIN_PASSWORD="smoke-new-$(openssl rand -hex 6)"

# --- JSON helpers -----------------------------------------------------------
jget() { python3 -c 'import sys,json;d=json.load(sys.stdin);print(eval("d"+sys.argv[1]))' "$1"; }

# --- Boot -------------------------------------------------------------------
echo "::group::compose up"
docker compose config -q
docker compose up -d --build db redis api relay
for svc in db redis api relay; do
  echo "waiting for $svc..."
  for _ in $(seq 1 60); do
    status=$(docker compose ps --format json "$svc" 2>/dev/null \
      | python3 -c 'import sys,json
try:
  d=json.load(sys.stdin)
  print(d[0]["Health"]) if isinstance(d,list) else print(d["Health"])
except Exception:
  print("")' || true)
    case "$svc" in
      db|redis) [ "$status" = "healthy" ] && break ;;
      api)      [ "$status" = "healthy" ] && break ;;
      relay)    docker compose exec -T relay wget -qO- http://localhost:8082/health >/dev/null 2>&1 && { status=ready; } ;;
    esac
    sleep 2
  done
  [ "$status" = "healthy" ] || [ "$status" = "ready" ] || { echo "FAIL: $svc not healthy"; docker compose logs "$svc" | tail -40; exit 1; }
done
echo "::endgroup::"

api() { docker compose exec -T api wget -qO- --header="Content-Type: application/json" "$@"; }

echo "== health =="
api http://localhost:8080/api/v1/health | jget '["status"]' | grep -q ok

echo "== bootstrap login (must_change_password flag) =="
login1=$(api --post-data="{\"username\":\"admin\",\"password\":\"$BOOTSTRAP_ADMIN_PASSWORD\"}" http://localhost:8080/api/v1/auth/login)
echo "$login1"
access1=$(echo "$login1" | jget '["tokens"]["access_token"]')
[ -n "$access1" ]
echo "$login1" | jget '["user"]["must_change_password"]' | grep -q True

echo "== forced password change =="
api --post-data="{\"old_password\":\"$BOOTSTRAP_ADMIN_PASSWORD\",\"new_password\":\"$NEW_ADMIN_PASSWORD\"}" \
    http://localhost:8080/api/v1/auth/password | jget '["message"]' | grep -qi changed

echo "== re-login with new password (flag must clear) =="
login2=$(api --post-data="{\"username\":\"admin\",\"password\":\"$NEW_ADMIN_PASSWORD\"}" http://localhost:8080/api/v1/auth/login)
access2=$(echo "$login2" | jget '["tokens"]["access_token"]')
[ -n "$access2" ]
echo "$login2" | jget '["user"]["must_change_password"]' | grep -q False

auth() { docker compose exec -T api wget -qO- --header="Content-Type: application/json" --header="Authorization: Bearer $access2" "$@"; }

echo "== create device (stream_url must NOT appear in API response) =="
dev=$(auth --post-data='{"name":"ci-camera","device_type":"camera","ip_address":"10.0.0.99","port":554}' http://localhost:8080/api/v1/devices)
dev_id=$(echo "$dev" | jget '["id"]')
[ -n "$dev_id" ]
echo "$dev" | python3 -c 'import sys,json;d=json.load(sys.stdin);assert "stream_url" not in json.dumps(d), "stream_url leaked!";assert d.get("stream_configured") in (False,None);print("ok: no stream_url in response")'

echo "== mint stream token =="
tok=$(auth --post-data="{\"device_id\":\"$dev_id\"}" http://localhost:8080/api/v1/stream/token)
stoken=$(echo "$tok" | jget '["token"]')
[ -n "$stoken" ]
echo "$tok" | jget '["expires_in"]' | grep -q . 

echo "== relay: valid token resolves device & starts session (via DB lookup) =="
resp=$(docker compose exec -T relay wget -qO- --header="Content-Type: application/json" \
    --header="Authorization: Bearer $stoken" \
    --post-data='{"channel":1}' http://localhost:8082/start)
echo "$resp" | jget '["hls_url"]' | grep -q '^/stream/hls/'

echo "== relay: bogus token rejected =="
noresp=$(docker compose exec -T relay sh -c \
  'wget -qO- --server-response --header="Content-Type: application/json" --header="Authorization: Bearer bogus.token.value" --post-data="{\"channel\":1}" http://localhost:8082/start 2>&1 || true')
echo "$noresp" | grep -qE "401|403|Unauthorized"

echo "== audit log contains login + device.create =="
audit=$(auth http://localhost:8080/api/v1/audit-logs)
echo "$audit" | grep -q '"action":"auth.login"'
echo "$audit" | grep -q '"action":"device.create"'

echo
echo "ALL SMOKE TESTS PASSED"