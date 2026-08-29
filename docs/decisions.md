# Decisions

Append-only. One entry per decision that would otherwise get re-litigated at 3am.
Newest at the bottom. Keep each to a few lines.

---

### 2026-08-29 — No project name yet

We're deliberately not naming the project before we know what it is. Naming early makes
an idea feel decided. `playa` is the container, not the product.

### 2026-08-29 — One folder per experiment, copy before abstracting

Shared helpers extracted early are how a playground becomes a codebase nobody wants to
touch. Duplicate freely; extract on the third occurrence.

### 2026-08-29 — Kill criteria are written before the code

Written into every experiment README by the template. Deciding what failure looks like
while calm is worth more than deciding it while tired and invested.

### 2026-08-29 — GitHub org `playa-hq`, repo `playground`

`playa` was taken. An org (rather than a personal repo) so the code isn't hostage to one
account, teammates are added once, and integrations install at org scope for every future
experiment repo. Repo starts private.

### 2026-08-29 — Entire org in the EU region; agent hooks stay personal

Org and project created under the `eu` jurisdiction — the event and the team are in
Barcelona, so session data stays in the EU. `.entire/settings.json` is committed and is
agent-neutral; `.claude/`, `.codex/`, `.cursor/` etc. are gitignored so each of us runs
`entire enable --agent <ours>` and nobody's choice is forced on anyone else.

### 2026-08-29 — AGENTS.md is the instruction file; tool-specific names are generated

Agent guidance lives in a committed, tool-neutral `AGENTS.md`. Files named for a
particular agent (`CLAUDE.md`, `GEMINI.md`, `.cursorrules`) are gitignored and
written by `scripts/setup.sh` as one-line pointers to it. Keeps the repo free of
any agent's name while still being read automatically by whichever tool you use.

### 2026-08-29 — Commit to main; branch only on collision

Experiments are isolated by folder, so three people on main rarely conflict, and
a single timeline makes `entire recap` a real standup. Entire's checkpoints are
per-branch, so work on a side branch is invisible to the team's search until it
merges. Branch when two people share an experiment, or before anything risky
near a demo.

### 2026-08-29 — deez.win deploys from CI, with the key confined by a forced command

The VPS has no Go toolchain, so CI builds and ships the binary, matching the
manual build → scp → restart flow the other services on that box use. The host
also runs live production, so the CI key is pinned behind an SSH forced command
that permits exactly two operations: writing the upload path and running the
deploy script. A shell, another path, or any forwarding is refused — verified.

Uploads are checksummed because a truncated transfer on a slow link produced a
binary that installed cleanly and then died with SIGBUS. The deploy script
verifies the hash, keeps the previous binary, and rolls back if the service does
not come up.

### 2026-08-29 — Worktrees and PRs, superseding commit-to-main

Earlier guidance was to commit straight to `main`, on the reasoning that folder
isolation makes conflicts rare and one timeline makes `entire recap` a real
standup. That is now superseded: everyone, people and agents alike, works in a
git worktree on a branch and lands through a PR.

Worktrees let several agents run at once without fighting over a single checkout
or over `git stash`. The cost is that Entire's checkpoints are per-branch, so
reasoning stays invisible to the team's search until a branch merges — which is
the argument for keeping branches short rather than for avoiding them.

`.github/workflows/check.yml` gates every PR on gofmt, vet, test, build, shell
syntax for the deploy scripts, and an obvious-credential scan. Deploys stay on
`main` only, so a PR can prove it builds but can never ship.
