#!/usr/bin/env bash
# Scaffold a new mini-MVP from the template.
#   ./scripts/new-experiment.sh voice-notes
set -euo pipefail

NAME="${1:-}"
if [ -z "$NAME" ]; then
  echo "usage: $0 <experiment-name>" >&2
  exit 1
fi

DEST="experiments/$NAME"
[ -e "$DEST" ] && { echo "$DEST already exists" >&2; exit 1; }

cp -r experiments/_template "$DEST"
sed -i.bak "s/{{NAME}}/$NAME/g; s/{{DATE}}/$(date +%Y-%m-%d)/g" "$DEST/README.md"
rm -f "$DEST/README.md.bak"

echo "==> Created $DEST"
echo "    Fill in $DEST/README.md (the pitch, the bet, the kill criteria) before writing code."
