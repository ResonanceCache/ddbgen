# NOTES

Decision log and known gaps, maintained during the initial build. One line of
rationale per judgment call, per the handoff spec.

## Decisions

- **Module path**: the repo was created directly under `github.com/ResonanceCache/ddbgen`,
  so the `OWNER` placeholder swap specified in the handoff happened at creation time
  rather than as a pre-publish sed.
- **Physical key attribute names** are hardcoded v1 convention: `pk`, `sk`, and
  `gsi1pk`/`gsi1sk`-style names derived by lowercasing the GSI name. Overridable later.
- **Delimiter** is hardcoded `#` in v1 (per-table configuration is a v2 item).

## Known gaps

(none yet — populated as milestones land)
