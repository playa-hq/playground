#!/usr/bin/env bash
# One-shot bootstrap for the playa toolchain.
# Idempotent: safe to re-run. Installs nothing that is already present.
set -euo pipefail

BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
export PATH="$BIN_DIR:$PATH"

info() { printf '\033[0;34m==>\033[0m %s\n' "$1"; }
warn() { printf '\033[0;33m !\033[0m %s\n' "$1"; }
ok()   { printf '\033[0;32m ✓\033[0m %s\n' "$1"; }

need_git() {
  git rev-parse --git-dir >/dev/null 2>&1 || { warn "not inside a git repo — run 'git init' first"; exit 1; }
}

# --- Entire: session capture wired into the git workflow -------------------
info "Entire CLI"
if command -v entire >/dev/null 2>&1; then
  ok "already installed ($(entire version | head -1))"
else
  curl -fsSL https://entire.io/install.sh | bash
  ok "installed to $BIN_DIR/entire"
fi

# --- fal.ai: genmedia ------------------------------------------------------
info "genmedia CLI (fal.ai)"
if command -v genmedia >/dev/null 2>&1; then
  ok "already installed (v$(genmedia version 2>/dev/null | sed -n 's/.*"version": "\([^"]*\)".*/\1/p'))"
else
  warn "not found — install it, then re-run this script"
  warn "  https://fal.ai  →  genmedia install instructions"
fi

# --- Aikido: local vulnerability scanner -----------------------------------
info "Aikido local scanner"
if command -v aikido-local-scanner >/dev/null 2>&1; then
  ok "native binary on PATH"
elif command -v docker >/dev/null 2>&1; then
  ok "docker available — scans run via aikidosecurity/local-scanner"
else
  warn "neither the aikido-local-scanner binary nor docker found"
  warn "  grab the binary from https://app.aikido.dev/settings/integrations/localscan"
fi

# --- agent pointers ------------------------------------------------------
# AGENTS.md is the committed, tool-neutral instruction file. Some agents only
# look for their own filename, so drop a local pointer for whichever are
# installed. These are gitignored — the repo stays agent-agnostic.
info "Agent instructions"
pointer() {
  local file="$1" bin="$2"
  command -v "$bin" >/dev/null 2>&1 || return 0
  if [ -f "$file" ]; then
    ok "$file already present"
    return 0
  fi
  printf '# Agent instructions live in AGENTS.md so every tool reads the same file.\n@AGENTS.md\n' > "$file"
  ok "wrote $file -> AGENTS.md"
}
pointer CLAUDE.md claude
pointer GEMINI.md gemini
[ -f AGENTS.md ] || warn "AGENTS.md is missing — read it before changing anything"

# --- env -------------------------------------------------------------------
info "Environment"
if [ -f .env ]; then
  ok ".env present"
else
  cp .env.example .env
  ok "created .env from .env.example — fill in your keys"
fi

cat <<'NEXT'

Next steps
  1. Fill in .env (FAL_KEY, OPENAI_API_KEY, AIKIDO_API_KEY)
  2. genmedia setup            # store your fal.ai key
  3. entire login              # authenticate
  4. entire enable             # install git + agent hooks in this repo
  5. ./scripts/new-experiment.sh my-idea

NEXT
