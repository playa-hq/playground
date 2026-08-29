# deez.win design system

## Idea

**A field guide to facts after dark.** Deez.win makes a verified data graph feel
like a place to explore, rather than a sterile dashboard or a retro arcade.
Every screen sits on the same near-black field with layered charcoal surfaces.
Mint marks action and live state, amber marks score, and coral marks errors.

## Bet

Players will trust the data-first premise more when the interface feels calm,
legible and spatially grounded. The visual energy should make turns feel alive
without competing with the fact, source and answer.

## Kill criteria

- [ ] The visual treatment makes a question or a source harder to scan than the
      previous simple theme.
- [ ] The logo or background reads as decoration rather than reinforcing the
      entity-graph premise.
- [ ] The typography loses clarity on a phone-sized screen.

## Tokens

| Role | Token | Use |
|---|---|---|
| Night | `--bg` `#07080A` | every page background |
| Charcoal | `--panel` `#101216` | cards and menus |
| Raised | `--panel-2` `#191C22` | interactive surfaces |
| Mint | `--accent` `#72F1C6` | primary action, focus and live state |
| Amber | `--gold` `#FFC857` | score and earned emphasis |
| Coral | `--danger` `#FF6B81` | errors and misses |
| Paper | `--paper` `#F1EEE7` | sourced answer receipts only |
| Display | Excalifont | logo, question prompt, headings |
| Reading/data | DM Mono | controls, values, body copy |

## Rules

- Use the six-sided die mark only with the `deez.win` wordmark or as a small
  home affordance.
- A screen gets one primary action. It is filled mint; all alternatives stay
  transparent or surfaced.
- Facts and citations use the paper receipt treatment, never a celebratory treatment. They
  are evidence, not a reward.
- Color is semantic and sparse: mint for action/live, amber for score, coral
  for error. Player dots may keep their identity color.
- Layouts collapse to one column before controls get crowded; touch targets
  stay at least 44px high on phones.
- Keep operational copy in DM Mono; reserve Excalifont for hierarchy and
  questions that people need to read quickly.
