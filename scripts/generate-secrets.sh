#!/usr/bin/env bash
# generate-secrets.sh — Generates cryptographically random secrets for CamGuid
# and writes them to .env (from .env.example). Safe to re-run: it preserves any
# existing values already present in .env.
#
# Usage:  ./scripts/generate-secrets.sh   (run from the repository root)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if [[ ! -f .env.example ]]; then
  echo "error: .env.example not found in $(pwd)" >&2
  exit 1
fi

[[ -f .env ]] || cp .env.example .env

random_hex() { openssl rand -hex "$1"; }

fernet_key() {
  # Prefer cryptography when available; otherwise build a Fernet-compatible
  # (url-safe base64 of 32 random bytes) key from openssl.
  if python3 -c 'import cryptography' 2>/dev/null; then
    python3 -c 'from cryptography.fernet import Fernet; print(Fernet.generate_key().decode())'
  else
    openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n'
  fi
}

set_or_replace() {
  # set_or_replace KEY VALUE  — writes or replaces KEY=... in .env
  local key="$1" value="$2"
  if grep -qE "^${key}=" .env; then
    sed -i.bak -E "s|^${key}=.*|${key}=${value}|" .env
  else
    printf '\n%s=%s\n' "$key" "$value" >> .env
  fi
  rm -f .env.bak
}

echo "Generating secrets for CamGuid..."

set_or_replace DB_PASSWORD "$(random_hex 16)"
set_or_replace JWT_SECRET "$(random_hex 32)"
set_or_replace ENCRYPTION_KEY "$(random_hex 32)"
set_or_replace CAMGUID_STREAM_SIGNING_SECRET "$(random_hex 32)"
set_or_replace NVR_STREAM_SIGNING_SECRET "$(random_hex 32)"
set_or_replace NVR_ENCRYPTION_KEY "$(fernet_key)"

echo
echo "Done. .env written with fresh secrets."
echo "Next:  docker compose up --build -d"
echo
echo "Keep the ROOT .env safe and rotate periodically (see SECURITY.md -> Secret Rotation)."