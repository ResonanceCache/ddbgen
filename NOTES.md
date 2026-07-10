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

- **Template grammar strictness**: each `#`-separated segment must be entirely a
  literal or entirely one placeholder (`ORD{ID}` is rejected). Stricter than the
  spec strictly requires, but it makes collision/boundary analysis exact.
- **Parser is purely syntactic** (go/parser + go/packages without type checking).
  Field types are recorded as source expressions; named type aliases over
  supported types are therefore not recognized in key placeholders. Fast, and
  works on single files for the golden corpus.
- **upper/lower decode inverse** is the identity: the stored (normalized) form is
  canonical, so the round-trip property tested is idempotence of encode∘decode.
- **hex fixed-width** is only claimed for `[N]byte` array fields; `[]byte` is
  variable-width and thus never range-eligible or valid before a non-final
  sk position.
- **Bare `sk.gt`/`sk.gte`/`sk.lt`/`sk.lte`/`sk.between` markers** declare intent for
  the docs matrix; the generated query is the same as a condition-less pattern
  (implicit entity prefix) and the range is applied through the generated
  boundary methods. Valued forms compile to static range conditions.
- **Patterns without an sk condition get an implicit begins_with** on the sk
  template's leading literal run (ElectroDB-style entity scoping), so a bare
  pattern on a shared partition can never unmarshal foreign entities.
- **Range-cut bounds are two-sided in shared partitions**: with a non-empty
  static prefix P, After(v) compiles to `BETWEEN P+enc(v)+"￿" AND P+"￿"`
  and Before(v) to `BETWEEN P AND P+enc(pred(v))+"￿"` (per-encoder
  predecessor functions in runtime). One-sided `<`/`>` conditions with a
  shared pk would scoop foreign entities that sort adjacent to the prefix
  (e.g. `PAY#...` sorts after `ORDER#...`). With an empty prefix the collision
  check already guarantees entity exclusivity and plain conditions are used.
  Before(v)'s predecessor underflow (nothing below v) compiles to the provably
  empty range `BETWEEN P AND P`.
- **Range methods are generated only for the first placeholder after the static
  prefix**, matching the spec's legal-cut list; deeper cuts require extending
  the pattern's sk condition.
- **Pattern queries trust key bounds instead of filtering on the entity-type
  attribute at read time**: DDB001 collision analysis plus two-sided bounds make
  cross-entity leakage structurally impossible, and a runtime `_et` filter would
  hide generator bugs rather than surface them.
- **Partition queries are generated only for pk-template groups shared by two
  or more entities** (structural equality of literals + encoders, field names
  ignored). A single-entity "collection" is just that entity's pattern query.
- **Batch writes bypass optimistic locking** (items are written with their
  current version as-is); the generated godoc says so. Chunk/retry caps follow
  the spec: 100/25 per request, 5 attempts, exponential backoff with full jitter,
  then ErrUnprocessedRemain (BatchGet also returns the partial result set).
- **Runtime package is ~830 lines, over the 500-line budget.** The overage is
  the per-encoder decode inverses and predecessor functions plus the two-sided
  range-bound constructors — correctness-critical logic that would otherwise be
  duplicated into every generated package. Chose one tested copy over the line
  budget.
- **Update setters are skipped for fields appearing in multi-placeholder GSI
  templates**: a lone setter cannot recompute such a key attribute. Setting
  those fields requires Put. (Sole-placeholder GSI fields get setters that
  resync the index key; that is the drift-prevention claim.)
- **Add methods are generated for integer fields only** (not floats), and not
  for GSI-templated or key fields.
- **DDB003 narrowed from the handoff spec's wording (deviation).** The spec says
  any non-final variable-width sk placeholder must error, but the spec's own
  worked example (Payment sk `PAY#{OrderID}#{PaymentID}`) violates that rule,
  and the worked example is load-bearing for the example app and tests. The
  narrowed rule errors only where lexicographic order is actually relied on:
  placeholders that range conditions cut through. Equality ops and
  boundary-aligned begins_with cuts stay exact for mid-key variable-width
  segments because the delimiter terminates every segment; range methods are
  simply not generated for variable-width placeholders.
- **x/tools pinned to v0.29.0** (and x/sync v0.10.0) to keep the module's minimum
  Go language version at 1.23 per the handoff spec.

- **Put semantics with version=**: `Put<E>` treats a zero version as "create"
  (condition `attribute_not_exists(pk)`) and any other value as "replace at
  exactly that version"; the struct's version field is advanced in place on
  success and restored on failure. `Put<E>IfNotExists` writes version 1 when the
  field is zero. `Update.Run` always increments (`if_not_exists(v, 0) + 1`),
  requires the item to exist, and only enforces a version match when
  `ExpectVersion` was called — the caller may not have read the item first.
- **sk.eq must consume the entire sk template** (DDB002); a prefix equality can
  never match a stored key, and begins exists for prefixes.
- **`ddbgen docs` writes ACCESS_PATTERNS.md into the annotated package
  directory; `ddbgen infra` writes into --out (default infra/) relative to the
  working directory; snapshots live next to generated code.** Matches sqlc's
  outputs-near-inputs convention.
- **Integration suite covers the batch chunking paths (30 items > one chunk)
  but not the unprocessed-retry loop**: DynamoDB Local never returns
  unprocessed keys/items under test-scale load. The retry loop is unit-level
  logic in runtime/batch.go with the cap and jitter constants documented.

- **M8 stretch: TransactWrite + init shipped; CDK Go emitter skipped.** The
  transact helpers are a deliberately thin passthrough (no version conditions —
  documented in the generated godoc); the CDK emitter was cut for time and
  because CFN/TF cover the deployment story.

## Known gaps

- Pattern queries against `projection=keys_only` GSIs unmarshal only key and
  discriminator attributes; the generated method still returns full entity
  structs whose non-projected fields are zero. Declare `projection=all` (the
  default) for patterns that read item bodies.
- The batch unprocessed-retry path is not exercised end-to-end (see above).

- Valued `sk.lt` with a value that consumes the *entire* sk template includes an
  exact-match item at the boundary (BETWEEN is inclusive). Prefix values — the
  overwhelmingly common case — are exact. Not reachable through generated
  Before() methods, which use predecessor encodings.
