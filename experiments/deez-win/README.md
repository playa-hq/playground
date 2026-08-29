# deez.win

**Started:** 2026-08-29 · **Status:** iteration 1 · **Owner:** _tbd_ · **Live:** https://deez.win

A 2–4 player quiz where the questions are built from a **verified entity graph**
rather than written by a language model. Dice decide who picks the topic;
everyone else claims one of the sub-topics the graph actually holds data for.

## The pitch

Trivia games invent questions and get them wrong. This one reads them off Cala's
entity graph, so every question has a typed value and a citation behind it. The
game is fun because of the dice and the topic fight; it is *trustworthy* because
no model is ever asked what is true.

## The bet

That grounding the questions in a real graph is both **more correct** and **more
interesting** than prompting a model — because introspecting an entity tells you
exactly which questions are answerable, which is a better question generator
than a blank prompt.

## Kill criteria

Written before the code, binding afterwards.

- [ ] **Cala coverage is too thin.** If fewer than half of the topics players
      naturally type return a usable numeric axis, free-text topics die and we
      fall back to a curated picker. If even that is thin, the whole premise fails.
- [ ] **Sub-topic picking is boring.** If playtesters skip past the claim step
      without engaging, cut it and let the topic picker choose everything.
- [ ] **A round takes longer than 4 minutes.** Party games live or die on pace.

## What works right now

The full loop, end to end, verified by playing a 3-player game through the API:

```
lobby → rolling → topic → subtopics → building → quiz → results
```

- **Anonymous auth via D3BIT** — `POST /auth/anon` gives every player a name and
  colour with no signup. Email magic-link and Google sign-in upgrade the account
  and keep the score.
- **Dice roll** decides turn order; ties break on join order so the ordering is
  always strict.
- **Topic pick** by the roll winner, then each remaining player claims one of the
  **top 5 sub-topics** — read from Cala's introspection endpoint, ranked with
  numeric axes first because those make the best questions.
- **Questions** are higher/lower on a numeric axis (no model involved at all) or
  multiple choice from a property, with distractors drawn from sibling entities.
- **Scoring** is speed-weighted, and you score **half** on questions from the
  axis you personally claimed.
- **Every answer shows its source** — the fact and its citation, which is the
  informative half of the game.

## Architecture

**Fully server-rendered Go, with htmx 4.0 for the interactivity.** There is no
client-side state and no build step: `html/template` renders every screen, and
the browser holds nothing but the DOM htmx last swapped in.

Three htmx 4 specifics the code depends on:

- **`innerMorph` swaps.** The panel re-renders every second; morphing means the
  dice keep their animation and the topic input keeps focus and cursor position
  through the poll. With `innerHTML` this design would be unusable.
- **`hx-target:inherited`.** Inheritance is opt-in in htmx 4, so the target is
  declared once on `#panel` and every button inside it inherits — and a stray
  `hx-get` elsewhere on the page can't hijack the swap.
- **Errors swap by default.** 4xx/5xx responses are swapped in 4.0, so the
  server returns real HTML for failures and they render in place.

One thing that changed under us: **htmx 4 does not read response headers**
(`HX-Push-Url`, `HX-Redirect` and friends are gone — only request headers
remain). Entering a room is therefore a plain form POST with a `303` redirect,
which is better anyway: the URL is correct, refresh works, and it degrades to a
working app with JavaScript switched off.

The one JS file, `static/sfx.js`, only listens for `htmx:after:swap` and makes
noises. It is not load-bearing.

## Cala, for real

The graph is company-, people- and finance-shaped (SEC filings, registries,
credit reports), so a topic is a **set of entities** — "Big Tech", "Spanish
startups", "European banks" — not a trivia category. The pipeline:

1. `POST /v1/knowledge/query` resolves the topic to entities (fuzzy
   `GET /v1/entities?name=` fills in when the topic is a single thing).
2. Every entity is introspected in parallel; only axes shared by **≥3** of them
   become sub-topics, ranked by how good a quiz question they make
   (headcount and founding date first, then revenue-style metrics, then
   relationships like headquarters or industry, then free-text properties).
3. On build, one `POST /v1/entities/{id}` per entity fetches exactly the
   claimed axes. Each value arrives with its own source, which is what the
   receipt shows. Time-series metrics use the latest point.

Numbers become higher/lower; dates become "founded first"; strings and
relationships become multiple choice with the *other entities' values* as
distractors, so every option is a real answer to the same question.

Check coverage for a topic before putting it on stage:

```bash
CALA_API_KEY=… go run . -probe "Spanish fintechs"
```

It prints the entities, the axes offered, and the questions a round would ask,
with sources. This is the tool behind kill criterion #1.

Set the key without it touching a shell history, transcript or git:

```bash
./ops/set-cala-key          # hidden prompt → ../../.env and the VPS, restarts the service
./ops/set-cala-key --local  # just .env
```

`cala_test.go` runs the whole pipeline against a mock of the documented
response shapes, so parsing is verified without a key.

**Measured live (2026-08-29):** `knowledge/query` takes 35–60s on a fresh
set and ~4s once Cala has seen it; introspection ~5s per entity (run in
parallel); a value fetch under 1s. Concurrent queries return 429. So:

- Resolution runs **off the request path** — the room sits in the building
  screen with a status line and polls its way forward.
- Topic graphs are **cached per process**, and the suggested topics are
  **warmed at startup**, sequentially, so the demo topics answer instantly.
- One retry after 8s on a 429.

"Big Tech companies" resolves to Apple, Microsoft, Amazon, Meta, NVIDIA and
offers *Employee count · Founding date · Revenue · Net income · Industry*, with
SEC 10-Q, Revelio Labs and GLEIF as sources. Fuzzy `entities?name=` is only a
fallback: "car maker" finds "MUSCLE MAKER, INC.".

## Run it

```bash
./run.sh --dev     # local stand-in auth, offline question data — no keys needed
./run.sh           # real D3BIT auth
```

Then open <http://localhost:8080>. Open a second browser profile or an incognito
window to play against yourself.

Environment:

| Variable | Effect if unset |
|---|---|
| `CALA_API_KEY` | Falls back to offline fixtures — the game still plays end to end |
| `D3BIT_URL` | Defaults to `https://api.d3bit.com`; `-d3bit` overrides |

## Sound

The client synthesizes 8-bit blips in WebAudio, so there is sound with no assets
and no key. `scripts/gen-audio.sh` generates richer fal.ai chiptune versions into
`static/sfx/`; anything that lands there is picked up automatically.

Sounds are **generated once and committed**, not called at runtime — a game loop
cannot wait on an inference round-trip, and this way a round costs nothing.

## The look

An arcade cabinet that prints receipts. Press Start 2P for anything the
cabinet says (codes, scores, headers), JetBrains Mono for anything a person
reads. Mint means live/correct, gold means points, and the **receipt** — the
answer reveal and the end-of-round review — is the one paper-coloured thing on
screen, because the proof should look different from the play. A ten-second
gold bar drains at the same rate the server pays the speed bonus; keys 1–4
answer; the entities the graph resolved are shown as chips so players see the
topic became something concrete.

## Design decisions worth knowing

**Cala decides what is true; the model only decides how it reads.** A numeric
axis becomes a higher/lower question straight off the graph, with no model in the
loop, so there is nothing to hallucinate. We resolve the axes *first* and build
questions from what exists, rather than inventing a topic and hoping for data.

**Polling, not WebSockets.** At 2–4 players a 1s poll is indistinguishable from a
socket and costs no reconnect logic. Revisit only if a round feels laggy.

**No database.** Rooms are ephemeral and in-memory. Persistence is the first
thing to add when the leaderboard needs to outlive the process.

**No build step, no client framework.** Templates live next to the handlers that
render them, so a change is one file and one reload. Codraw's Vue setup is the
house style; this deliberately isn't, because at 2am a toolchain is a liability.

## Known gaps

- **The UI has been rendered headlessly (home, axes, quiz, results) but not
  played by hand in a real browser.** htmx swaps, the bonus bar restart and
  the keyboard answers still want a human check.
- **A typed topic costs up to a minute the first time.** Cached and warmed
  topics are instant; anything new sits on the building screen. Whether
  players tolerate that is a playtest question, and it presses on the
  four-minute kill criterion.
- Kill criterion #1 (coverage) is now measurable with `-probe` but not yet
  measured across "topics players naturally type" — only on curated sets.
- **`htmx.org@4.0.0` is not the npm `latest` tag yet** (that still points at
  2.0.10), so the version in the CDN URL is pinned deliberately. Don't "fix" it
  to a range.
- **`POST /auth/anon` is 500ing on production D3BIT** as of 2026-08-29. `--dev`
  works around it; a local D3BIT may not have the problem.
- Numeric phrasing is naive: "higher founded year" should read "founded later",
  and negative years (BC) render as negative numbers.
- No topic-vote step or fal cover images yet — that was the original sketch's
  idea and is still worth building.

## Leaderboard

All-time standings persist to a JSON file (`-leaderboard`, default alongside the
binary), written through a temp file so a crash cannot truncate it. Rooms stay
in memory — they are throwaway — but a score is the one thing worth surviving a
deploy, and it gives the home screen the panel the original sketch always had.

Ties on top score count as a win for everyone who reached it: with 2-4 players
ties are common enough that breaking them arbitrarily reads as a bug.

## Notes

The sketch had `TOPIC` as a host-level setting at lobby creation. That was
dropped in favour of the dice-winner picking it in-room, because a hidden,
in-room pick is the moment the game comes alive — a setting on a form is not.
