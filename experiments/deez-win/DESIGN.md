# deez.win design system

## Idea

**A field guide to facts after dark.** Deez.win makes a verified data graph feel
like a place to explore, rather than a sterile dashboard or a retro arcade.
The system uses a deep teal night field, luminous chartreuse for actions and
truth, and an amber signal for scores and room state.

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
| Night field | `--night` | page depth and inputs |
| Glass surface | `--surface` | cards and contained information |
| Verify/action | `--mint` | primary actions, focus, sourced facts |
| Score/signal | `--sun` | scores, room identifiers, warnings |
| Error | `--coral` | invalid or incorrect states |
| Display | Excalifont | logo, question prompt, headings |
| Reading/data | DM Mono | controls, values, body copy |

## Rules

- Use the hexagonal graph mark only with the `deez.win` wordmark or as a small
  home affordance. Its node is amber; its graph lines are mint.
- A screen gets one primary action. It is filled mint; all alternatives stay
  transparent or surfaced.
- Facts and citations get a mint left rule, never a celebratory treatment. They
  are evidence, not a reward.
- The topographic background stays low contrast and must never carry text.
- Keep operational copy in DM Mono; reserve Excalifont for hierarchy and
  questions that people need to read quickly.
