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

### 2026-08-29 — A Cala topic is a set; axes must be shared by three entities

Cala's graph is entity-shaped (companies, people, countries, filings), so a
player's topic is resolved through the query endpoint to a *set* of entities,
not matched to a category. A sub-topic is only offered when at least three of
those entities have it: an axis one entity holds cannot become a "which of
these" question. Ranking prefers numbers people have intuitions for (headcount,
founding date, revenue) over accounting line items and addresses.

Distractors for fact questions are the other entities' values on the same axis,
never entity names — the earlier code could offer "Stripe" as an answer to
"headquarters?".

The `-probe` flag prints what a topic resolves to; it is the measurement behind
the experiment's first kill criterion and should be run before any topic is
suggested on the home screen.

### 2026-08-29 — Secrets are entered through a hidden prompt, never pasted

`ops/set-secret <NAME>` reads the key with a silent prompt and writes it to `.env`
and the VPS env file over SSH. A key pasted into an agent chat lands in the
session transcript Entire captures; a key on a command line lands in shell
history. Both would then need rotating.

### 2026-08-29 — Cala's rate limits shape the round: fetch per entity, not per axis

`POST /entities/{id}` allows about five calls a minute (measured: six
sequential calls all 429 in 0.3s, a 200 after a sixty-second pause) and a
second in-flight `knowledge/query` is refused. A round therefore keeps six
entities and fetches *all* ranked axes for an entity in one call, serially,
starting as soon as the axes exist — the fetch runs under the claiming phase
rather than after it. Per-axis fetching on claim, which is the more obvious
shape, would cost entities × axes calls and never finish inside the limit.

The same limit is why loading is shown as a graph rather than a spinner: with
a minute of real waiting in the round, players should see what is arriving.

### 2026-08-29 — Cover art is a two-model chain, cut out to transparency

FLUX schnell draws the subject on a flat white ground; BiRefNet cuts it out to
an RGBA PNG. Asking a generator for "transparent background" does not work —
diffusion models emit opaque pixels — so transparency is a second, cheap
segmentation step rather than a prompt. Covers are decorative and cached per
topic; a fal failure degrades to the opaque image, then to no image, and never
touches the round.

### 2026-08-29 — Account lives in the top-right menu

Sign-in was a card in the grid competing with the game. It is now a `<details>`
menu on the top bar (no JS): a guest sees their name and "sign in", a signed-in
player sees their name and an email; the popover holds the magic-link form and
Google. Same markup on the room page minus the popover.
