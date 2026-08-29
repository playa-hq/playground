#!/usr/bin/env bash
# Run an Aikido vulnerability scan over this repo.
# Prefers the native binary; falls back to the official Docker image.
#
#   ./scripts/scan.sh                 # scan repo root
#   ./scripts/scan.sh experiments/foo # scan a subtree
set -euo pipefail

[ -f .env ] && set -a && . ./.env && set +a

TARGET="${1:-.}"
REPO_NAME="${AIKIDO_REPO_NAME:-$(basename "$(pwd)")}"
BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo main)"

if [ -z "${AIKIDO_API_KEY:-}" ]; then
  echo "AIKIDO_API_KEY is not set." >&2
  echo "Create one at https://app.aikido.dev/settings/integrations/localscan and put it in .env" >&2
  exit 1
fi

echo "==> Scanning '$TARGET' as repo '$REPO_NAME' (branch: $BRANCH)"

if command -v aikido-local-scanner >/dev/null 2>&1; then
  exec aikido-local-scanner scan "$TARGET" \
    --apikey "$AIKIDO_API_KEY" \
    --repositoryname "$REPO_NAME" \
    --branchname "$BRANCH"
elif command -v docker >/dev/null 2>&1; then
  exec docker run --rm -v "$(pwd):/src" aikidosecurity/local-scanner \
    scan "/src/${TARGET#./}" \
    --apikey "$AIKIDO_API_KEY" \
    --repositoryname "$REPO_NAME" \
    --branchname "$BRANCH"
else
  echo "No scanner available: install the aikido-local-scanner binary or Docker." >&2
  exit 1
fi
