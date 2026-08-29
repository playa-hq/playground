#!/usr/bin/env bash
# Entry point for this experiment.
#
#   ./run.sh              real D3BIT auth, offline question data
#   ./run.sh --dev        local stand-in auth (no D3BIT needed)
set -euo pipefail
cd "$(dirname "$0")"

[ -f ../../.env ] && set -a && . ../../.env && set +a

ARGS=()
[ "${1:-}" = "--dev" ] && ARGS+=(-dev-auth)

go build -o deezwin . && exec ./deezwin -addr "${ADDR:-:8080}" "${ARGS[@]}"
