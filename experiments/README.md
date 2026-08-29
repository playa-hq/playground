# experiments

One directory per mini-MVP. Nothing in here is precious — that's the point.

```bash
../scripts/new-experiment.sh my-idea
```

Rules of the room:

1. **Every experiment starts with a README, not with code.** The pitch, the bet,
   and the kill criteria go in first. Ten minutes of writing saves two hours of building.
2. **Kill criteria are binding.** If you hit them, stop. Move the folder to `experiments/`
   history and write down what you learned.
3. **One command to run it.** `./run.sh`. If a teammate can't run it in thirty seconds,
   it doesn't demo.
4. **Nothing here is shared infrastructure.** If two experiments need the same helper,
   copy it. Extract only when a third one shows up.

## Log

| Experiment | Started | Status | Verdict |
|---|---|---|---|
| [world-game](world-game/) | 2026-08-29 | iteration 0 | playable end to end; UI unreviewed |
