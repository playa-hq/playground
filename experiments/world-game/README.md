# world-game

**Started:** 2026-08-29 · **Status:** iteration 0 · **Owner:** _tbd_

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

## Design decisions worth knowing

**Cala decides what is true; the model only decides how it reads.** A numeric
axis becomes a higher/lower question straight off the graph, with no model in the
loop, so there is nothing to hallucinate. We resolve the axes *first* and build
questions from what exists, rather than inventing a topic and hoping for data.

**Polling, not WebSockets.** At 2–4 players a 1s poll is indistinguishable from a
socket and costs no reconnect logic. Revisit only if a round feels laggy.

**No database.** Rooms are ephemeral and in-memory. Persistence is the first
thing to add when the leaderboard needs to outlive the process.

**No build step on the frontend.** Plain HTML/CSS/JS so anyone on the team can
edit it at 2am without a toolchain in the way. Migrate to Vue when it hurts.

## Known gaps

- **The UI has not been checked in a browser** — it was built and the API was
  tested, but nobody has looked at it yet. Do that first.
- **`POST /auth/anon` is 500ing on production D3BIT** as of 2026-08-29. `--dev`
  works around it; a local D3BIT may not have the problem.
- Numeric phrasing is naive: "higher founded year" should read "founded later",
  and negative years (BC) render as negative numbers.
- No topic-vote step or fal cover images yet — that was the original sketch's
  idea and is still worth building.
- No leaderboard across games; scores die with the room.

## Notes

The sketch had `TOPIC` as a host-level setting at lobby creation. That was
dropped in favour of the dice-winner picking it in-room, because a hidden,
in-room pick is the moment the game comes alive — a setting on a form is not.
