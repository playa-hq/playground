# Working in this repo

Read this before making changes. It is short on purpose.

This is a hackathon playground: small experiments, one folder each, most of them
thrown away. Optimise for a working demo, not for a codebase.

## Hard rules

**Never put agent attribution in anything that lands in git.** No
`Co-Authored-By` trailers, no "generated with <tool>" lines in commit messages,
code comments, or docs. Write everything as if the team wrote it. Session
capture is Entire's job — it records that separately, on its own refs, and the
git history stays clean.

**Never expose `-dev-auth`.** It mints a session for anyone who asks, with no
verification. It exists so nobody is blocked when D3BIT is unreachable. Local
only, always.

**Never commit a key.** `.env` is gitignored; keep it that way. If one lands in
a commit, rotate it — deleting the line is not enough.

## Before your first change

```bash
make setup                     # toolchain check, creates .env
entire login
entire enable --agent <yours>  # claude-code, codex, cursor, gemini…
```

Agent hook directories (`.claude/`, `.codex/`, `.cursor/`) are gitignored, so
everyone picks their own tool. `.entire/settings.json` is shared and committed.

If `entire checkpoint list` stays at zero after a turn, your hooks did not load
— they are read at agent startup, so restart your agent after `entire enable`.

## How work is structured

One folder per experiment under `experiments/`, self-contained, with a `run.sh`
that starts it in one command.

**Write the README before the code.** The template makes you state the pitch,
the bet, and the kill criteria first. Kill criteria are binding: if you hit
them, stop and write down what you learned.

**Copy before you abstract.** Two experiments needing the same helper is a
coincidence. Extract on the third.

## The architectural rule that matters

In `world-game`, and anywhere else data meets a model:

> **The data source decides what is true. The model only decides how it reads.**

Question values come from Cala's entity graph — introspect an entity to learn
which axes exist, then build from those. Never ask a model to invent a fact and
never invent a topic and hope the data exists. If something cannot be grounded,
drop it rather than fill the gap with plausible text.

## Before you push

```bash
gofmt -w <changed .go files> && go vet ./... && go build ./...
make scan          # Aikido, needs AIKIDO_API_KEY
```

Run the thing you changed. For a game, that means playing a round — compilation
is not verification. A build passing only proves the code compiles, not that
your edit landed where you intended.

## Shared files that conflict

`experiments/README.md` (the log table) and `docs/decisions.md` are append-only
and everyone touches them. Expect conflicts; they are always trivial — keep both
sides. Add your entry at the end rather than mid-file.
