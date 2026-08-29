# Stack reference

Deeper notes on each tool than the README carries. Read the README first.

---

## Entire — session capture on the git workflow

**What it does.** Entire hooks into git and into your coding agent, and records the
agent sessions that produced each push. Sessions are indexed alongside commits, so the
repo ends up holding not just the code but the reasoning that got it there. On a
hackathon team where four people are building in parallel and nobody has time to write
up what they tried, that record *is* the handover.

It is agent-agnostic — it supports Claude Code, Codex, Gemini CLI, Cursor, OpenCode and
others through the same hook mechanism.

**Install**

```bash
curl -fsSL https://entire.io/install.sh | bash   # Linux / macOS → ~/.local/bin
brew install --cask entire                        # macOS, via entireio/tap
scoop install entire/entire                       # Windows
go install github.com/entireio/cli/cmd/entire@latest
```

Requires git 2.25+. Config lives in `~/.config/entire`, cache in `~/.cache/entire`.

**Set up in the repo**

```bash
entire login              # device auth
entire enable             # install git hooks + create .entire/
entire agent add <name>   # wire up a specific agent's hooks
entire status             # what's tracked right now
```

`entire enable` creates `.entire/settings.json` (committed, shared) and
`.entire/settings.local.json` (gitignored, personal). Session metadata is written to a
separate `entire/checkpoints/v1` branch, so it never clutters your working history.

**Day to day**

| Command | Use it for |
|---|---|
| `entire search <query>` | Semantic + keyword search across checkpoints, commits and sessions |
| `entire recap` | Summarize recent checkpoint activity |
| `entire dispatch` | Generate a summary of recent agent work — good for standup |
| `entire activity` | Your own activity overview |
| `entire session resume` | Switch branches and restore the latest checkpointed session |
| `entire doctor` | Fix stuck sessions |
| `entire agent-help` | Machine-readable usage, always matching the installed CLI |

Set `ENTIRE_NO_AUTO_UPDATE=1` in CI to suppress update prompts.

Docs: <https://docs.entire.io>

---

## fal.ai — `genmedia`

**What it does.** `genmedia` is an agent-first CLI over fal.ai's model catalogue: image,
video, audio and speech generation, plus model discovery, schemas and pricing. "Agent-first"
means every command speaks JSON and has a non-interactive mode, so it drops into a script
or an agent loop without a TTY.

**Set up**

```bash
genmedia setup                                     # interactive
genmedia setup -y --api-key "$FAL_KEY"             # CI / agents
```

It will auto-load `FAL_KEY` from a local `.env` unless you pass `--no-auto-load-env`.
Get a key at <https://fal.ai/dashboard/keys>.

**The loop that matters**

```bash
genmedia models --search "image to video"          # find a model
genmedia schema fal-ai/flux/dev                    # what does it take?
genmedia pricing fal-ai/flux/dev                   # what does it cost?
genmedia run fal-ai/flux/dev --download out/       # run it, save the output
genmedia run "a lighthouse in fog, 35mm"           # or let it route for you
```

Long jobs: `--async` returns a `request_id`, then `genmedia status <id>`. Add `--logs` to
stream progress. `genmedia gallery` builds a local HTML contact sheet of everything a
session generated — a `file://` URL, no server — which is the fastest way to review a
batch of outputs.

`genmedia skills install` drops a skill bundle for your coding agent so it can drive the
CLI without you reciting flags.

**Cost discipline.** Video generation is the expensive one. Check `genmedia pricing`
before you fan out a batch, and prototype on cheap image models before you commit to
video.

---

## Aikido — repository vulnerability scanning

**What it does.** Scans the repo for vulnerable dependencies, leaked secrets, SAST
findings, licence problems and IaC misconfigurations. The local scanner runs entirely on
your machine — code never leaves — and uploads only the findings.

**Get a token.** <https://app.aikido.dev/settings/integrations/localscan> → generate.
It is shown **once**. Format `AIK_CI_xxx`. Put it in `.env` as `AIKIDO_API_KEY`.

**Run it**

```bash
make scan                      # wrapper — reads .env, infers repo + branch
./scripts/scan.sh experiments/foo
```

Under the hood, whichever is available:

```bash
# native binary (download from the Aikido UI)
aikido-local-scanner scan . --apikey "$AIKIDO_API_KEY" \
  --repositoryname playa --branchname main

# or docker
docker run --rm -v "$(pwd):/src" aikidosecurity/local-scanner \
  scan /src --apikey "$AIKIDO_API_KEY" --repositoryname playa --branchname main
```

x86_64 and ARM64. Needs ~10GB of temp space — the scanner works in a `.aikidotmp/`
folder, which is gitignored. Results land in the Aikido feed under the repository name
you passed.

**Why it's in a hackathon repo.** Demo code accretes dependencies fast, and API keys
end up pasted in the wrong file at 3am. One scan before you push is cheap insurance
against shipping a live key in a public repo.

---

## OpenAI

Reasoning, structured extraction, agent orchestration — the text side of whatever we
build, with fal.ai handling pixels and audio. Key goes in `.env` as `OPENAI_API_KEY`;
experiments read it through `run.sh`.
