# playa

> **playa** *(n.)* — a flat desert basin that floods, briefly, and then dries. Things
> grow there fast and most of them don't last. The ones that do, earn it.

A team playground for **Tech Europe Hackathon, Barcelona**.

We don't have a project yet. That is deliberate. This repo is where we build small
things quickly, find out which one has legs, and only then commit. Every idea lives in
its own folder under `experiments/`, comes with kill criteria written down *before* the
first line of code, and gets thrown away without ceremony when it doesn't work.

---

## Quick start

```bash
git clone <this-repo> && cd playa
make setup          # install + verify the toolchain, create .env
$EDITOR .env        # add your keys
make doctor         # confirm everything is wired up
make new NAME=my-idea
```

| Command | What it does |
|---|---|
| `make setup` | Install and check the toolchain. Idempotent — re-run it anytime. |
| `make doctor` | One-line status for every tool and for `.env`. |
| `make new NAME=x` | Scaffold `experiments/x` from the template. |
| `make scan` | Run an Aikido vulnerability scan over the repo. |

---

## How we work

**One folder per idea.** `experiments/<name>/`, self-contained, with a `run.sh` that
starts it in one command. If a teammate can't run your experiment in thirty seconds, it
won't survive a demo either.

**README before code.** The template makes you write three things first: the pitch, the
bet, and the kill criteria. Ten minutes of writing regularly saves two hours of building
the wrong thing at 2am.

**Kill criteria are binding.** You wrote them when you were calm. Trust that version of
yourself over the one at hour eighteen. When you kill something, write down what you
learned — the dead ends are worth more than they look.

**Copy before you abstract.** Two experiments needing the same helper is a coincidence.
Extract only when a third shows up. Shared infrastructure is how a playground turns into
a codebase you're afraid to touch.

**Scan before you push.** `make scan`. Cheap insurance.

```
playa/
├── experiments/       # one mini-MVP per folder — the actual work
│   └── _template/     # what `make new` copies
├── scripts/           # setup, scan, scaffold
├── docs/
│   ├── stack.md       # deep reference for every tool below
│   └── decisions.md   # what we chose and why — append-only
├── Makefile
└── .env.example
```

---

## The stack

Built on the hackathon's sponsor tooling. Full setup and usage notes in
**[`docs/stack.md`](docs/stack.md)** — the short version:

### Entire — session capture on the git workflow

[Entire](https://entire.io) hooks into git and into your coding agent, capturing the
sessions that produced each push. Sessions are indexed alongside commits, so the repo
records not only what changed but the reasoning behind it. On a team building four
things in parallel with nobody writing anything up, that record *is* the handover.
Agent-agnostic — Claude Code, Codex, Gemini CLI, Cursor and others all work through the
same hooks.

```bash
curl -fsSL https://entire.io/install.sh | bash
entire login && entire enable
entire search "why did we drop the websocket approach"
entire recap                # standup, generated
```

### fal.ai — `genmedia`

[fal.ai](https://fal.ai) is the generative media layer: image, video, audio, speech. The
`genmedia` CLI is agent-first — JSON output and a non-interactive mode on every command,
so it drops straight into a script or an agent loop.

```bash
genmedia setup                                # store your FAL_KEY
genmedia models --search "image to video"     # discover
genmedia pricing fal-ai/flux/dev              # check the damage first
genmedia run fal-ai/flux/dev --download out/  # generate
genmedia gallery                              # review the batch as a local HTML sheet
```

Video is the expensive one. Prototype on cheap image models, check pricing before you
fan out a batch.

### Aikido — repository vulnerability scanning

[Aikido](https://aikido.dev) scans for vulnerable dependencies, leaked secrets, SAST
findings and IaC misconfigurations. The local scanner runs on your machine — code never
leaves — and uploads only findings.

```bash
make scan
```

Needs `AIKIDO_API_KEY` (an `AIK_CI_*` token from
[the local-scan settings page](https://app.aikido.dev/settings/integrations/localscan);
it's shown once). Runs as a native binary or via `aikidosecurity/local-scanner` on
Docker — `scripts/scan.sh` picks whichever you have.

### OpenAI

Reasoning, structured extraction and agent orchestration — the text side of whatever we
build, with fal.ai handling pixels and audio.

---

## Keys and secrets

Everything goes in `.env`, which is gitignored. `.env.example` lists what's needed:

| Variable | Where to get it |
|---|---|
| `FAL_KEY` | <https://fal.ai/dashboard/keys> |
| `OPENAI_API_KEY` | <https://platform.openai.com/api-keys> |
| `AIKIDO_API_KEY` | <https://app.aikido.dev/settings/integrations/localscan> |

Never commit a real key. If one lands in a commit, rotate it — don't just delete the
line. `make scan` catches this, but only if you run it.

---

## Naming

There isn't a project name yet. There will be, once something here earns it. Until then
this is the playa: it floods, things grow fast, and most of it dries up. That's working
as intended.
