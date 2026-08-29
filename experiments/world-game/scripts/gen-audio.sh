#!/usr/bin/env bash
# Generate the 8-bit sound bed with fal.ai via the genmedia CLI.
#
# The client synthesizes fallback blips in WebAudio, so the game has sound
# without this. Run it to upgrade: anything that lands in static/sfx/ is picked
# up automatically on the next load, no code change.
#
# Sounds are generated once and committed as static assets rather than called
# at runtime — a game loop cannot wait on an inference round-trip, and this way
# a round costs nothing.
set -euo pipefail
cd "$(dirname "$0")/.."

command -v genmedia >/dev/null 2>&1 || { echo "genmedia not on PATH — see ../../docs/stack.md" >&2; exit 1; }

OUT=static/sfx
mkdir -p "$OUT"

# Keep these short: they fire on every interaction and long tails feel laggy.
gen() {
  local name="$1" prompt="$2"
  if [ -f "$OUT/$name.wav" ] && [ "${FORCE:-0}" != "1" ]; then
    echo "  = $name (exists, FORCE=1 to regenerate)"
    return
  fi
  echo "==> $name"
  genmedia run "$prompt" --download "$OUT/$name.{ext}"
}

gen roll    "8-bit chiptune dice rattle, short, retro arcade, 0.4 seconds, no music"
gen select  "8-bit chiptune UI blip, single short square wave note, 0.15 seconds"
gen correct "8-bit chiptune success arpeggio, bright ascending, 0.6 seconds"
gen wrong   "8-bit chiptune error buzz, descending sawtooth, 0.4 seconds"
gen win     "8-bit chiptune victory fanfare, triumphant, 1.5 seconds"

# Background loop. Longer, so it is generated last and is optional.
gen theme   "8-bit chiptune background loop, upbeat quiz show, seamless loop, 30 seconds"

echo
echo "Done. Files in $OUT — they are served at /sfx/<name>.wav and override"
echo "the WebAudio fallbacks automatically."
