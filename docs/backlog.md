# Backlog

Ideas we want to keep but are not building today. Append at the end; move an
entry into `experiments/` when it starts.

## Direction: deez.win is a collection of games

**Slogan:** *fun IQ-increasing multiplayer games.*

deez.win is the venue, not one game. The quiz (`experiments/deez-win`) is the
first table; the home page and lobby should eventually pick a game. Shared
across games: D3BIT identity, the leaderboard, the loading graph, and the rule
that a data source decides what is true.

## Game: Scavenger (Cala → camera → fal → three.js)

Cala hands the table a list of objects to find in the room they are in (an
office: a stapler, a plant, a monitor…). Players photograph the real things.
fal turns each photo into a 3D model. three.js drops the models into one
shared scene with gravity, and players smash them together.

Why it fits: the data source names the targets, the photo is the receipt, and
the payoff is physical and silly. Multiplayer because everyone's finds land in
the same scene.

Pieces, roughly in order of risk:

1. **Item list from Cala** — what does the graph hold about ordinary objects?
   Probe before anything else; if it is thin, the list can come from an
   entity like "office supplies" and its relationships, or fall back to a
   curated set. Kill criterion: fewer than ten playable objects.
2. **Photo → 3D** — fal image-to-3D (candidates: Hunyuan3D, TripoSR, Trellis
   endpoints on fal). Measure latency; the round design depends on whether
   this is 10s or 2 minutes. Cut-out first (BiRefNet, already wired) so the
   model gets a clean subject.
3. **Verifying the photo matches the item** — a vision check (fal or OpenAI)
   that says "yes, that is a stapler". Without it the game has no rules.
4. **Scene** — three.js + a physics engine (rapier or cannon-es), one room,
   models stream in as they finish, throw with a flick. Server-rendered app
   stays; this is one `<canvas>` page with a small bundle, the first client
   framework we would allow.
5. **Smashing** — collisions score; the leaderboard records it.

Unknowns to settle first: latency of photo→3D on fal, and whether Cala has
anything to say about staplers.
