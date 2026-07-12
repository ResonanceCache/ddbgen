# Changelog

All notable changes to ddbgen are documented here. This project follows
[semantic versioning](https://semver.org/); while it is `v0.x`, the generated
API surface may change between minor versions.

## [v0.1.1] — 2026-07-11

Two fixes found by dogfooding ddbgen against a real hand-written five-entity
single-table repository (migrating a parked project's DynamoDB layer). See
[the writeup](docs/blog/typed-single-table-dynamodb.md) for the story.

### Fixed
- **Escape-hatch `Scan`.** The `client.DynamoDB()` escape hatch returns the
  `runtime.DynamoDB` interface, which listed only the eight operations
  generated code calls — omitting `Scan`, the single most common operation an
  escape hatch exists to reach (admin counts, migrations, full-table audits).
  Added; the interface stays a tight, mockable nine operations.
- **Whole-key variable-width `sk.eq` / `sk.begins`.** An exact or prefix match
  on a GSI sort key that is a single variable-width placeholder (e.g.
  `{Category}`) was wrongly rejected. The key genuinely ends there, so the
  match is correct. `sk.gt` over such a placeholder now reports the more
  accurate `DDB003` (lexicographic-order) diagnosis instead of the
  begins-oriented `DDB002`.

## [v0.1.0] — 2026-07-10

First public release.

### Added
- Marker DSL (`//ddb:entity` / `key` / `index` / `pattern`) compiling
  annotated Go structs to a typed single-table client.
- Fixed-width key encoders with proven lexicographic ordering: `rfc3339`,
  `epoch`, `epochms`, `pad<N>`, `upper`, `lower`, `hex`, `ulid`, `urlenc`.
- Synthesized key attributes computed inside marshal/update, so GSI keys
  cannot drift from the data.
- Per-pattern typed queries with boundary-cut range methods
  (`<Field>After` / `Before` / `Between`), `iter.Seq2` streaming, and cursor
  pagination; a server-side entity-type filter makes typed results exact in
  shared partitions.
- CRUD with optimistic locking, typed update builders, item collections,
  batch helpers, and a `TransactWrite` passthrough.
- Static checks `DDB001`–`DDB008` (key collisions, pattern satisfiability,
  sortability, encoder/type compatibility, version/ttl typing, duplicate
  names, placeholder resolution, reserved-attribute shadowing).
- Snapshot-based breaking-change detection (`ddbgen diff`).
- CloudFormation / Terraform / `ACCESS_PATTERNS.md` emitters from the same
  parse.
- `ddbgen init` scaffolder; interface-based client injection; raw filter
  expressions; consistent reads; conditional deletes.

[v0.1.1]: https://github.com/ResonanceCache/ddbgen/releases/tag/v0.1.1
[v0.1.0]: https://github.com/ResonanceCache/ddbgen/releases/tag/v0.1.0
