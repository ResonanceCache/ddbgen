# Static checks

`ddbgen generate` and `ddbgen diff` run these checks after parsing and fail
with one line per finding: `file:line: DDB00N: message`. Checks are
conservative by design — when overlap cannot be ruled out statically, ddbgen
errors and asks for a disambiguating literal rather than guessing.

## DDB001 — key collision / ambiguity

Two entities on the same table (or the same GSI) could write items with
identical physical keys. Templates are compared as segment sequences: the
delimiter never occurs inside an encoded segment, so templates with
different segment counts are always distinct, and literal segments at the
same position must match exactly for an overlap to be possible. Any
position pairing a placeholder with anything is conservatively treated as
overlapping — `USER#{ID}` collides with `USER#{Email}`, while `USER#{ID}`
and `ORDER#{ID}` are provably disjoint.

A collision is reported only when the partition keys overlap **and** the
sort keys fail to disambiguate. Entities sharing a partition on purpose
(item collections) stay valid as long as their sk templates start with
distinct literals.

**Fix:** add a distinguishing literal segment (usually an entity-name
prefix) to one of the templates.

## DDB002 — pattern satisfiability

A `//ddb:pattern` must be executable against keys that are actually
written:

- the pattern's `pk` template must be structurally identical (same
  literals, fields, encoders, order) to the pk template of its declared
  index;
- sort-key conditions must align to a placeholder boundary of that index's
  sk template — `sk.begins="ORD"` is rejected because it cuts mid-literal,
  and a condition ending inside a *non-final* variable-width placeholder is
  rejected because it could match part of a value of the longer key (e.g.
  `sk.begins="PAY#{OrderID}"` over `PAY#{OrderID}#{PayID}`). A condition
  that ends at the template's *final* placeholder is fine even when that
  placeholder is variable-width — the key genuinely ends there, so an exact
  (`sk.eq`) or prefix (`sk.begins`) match on the whole attribute is correct
  (e.g. a GSI sort key that is a single `{Category}`). (A begins value that
  stops at a segment boundary without writing the trailing `#` —
  `sk.begins="ORDER"` — is accepted and canonicalized: the generated prefix
  always carries the delimiter, so range cuts and sibling literals like
  `ORDERX` behave correctly.);
- `sk.eq` must spell out the complete sort key (shorter keys are never
  written; use `sk.begins` for prefixes), and a `sk.begins` value that
  covers the whole sort key must not end with the delimiter (keys never
  end with one, so it could never match);
- valued `sk.gt/gte/lt/lte` must end with a placeholder — the range bound;
- patterns may not target `projection=keys_only` GSIs: the projected items
  carry no data attributes and no entity-type attribute, so a typed query
  would return zero-valued structs.

## DDB003 — sortability

Any sk-template placeholder that a range condition ranges over — whether a
valued `sk.gt/gte/lt/lte` in a marker or a bare
`sk.between/gt/gte/lt/lte` intent resolved to the first placeholder after
the leading literals — must have a fixed-width encoding (`rfc3339`,
`epoch`, `epochms`, `pad<N>`, `ulid`, `hex` of a `[N]byte` field).
Variable-width encodings break lexicographic range cuts: `"2"` sorts after
`"10"`.

Non-final variable-width placeholders that no range touches (like
`{OrderID}` in `PAY#{OrderID}#{PaymentID}`) are legal: the delimiter
terminates every segment, so equality operations and boundary-aligned
`begins_with` cuts remain exact regardless of segment widths. Range
refinement methods are simply not generated for variable-width
placeholders.

## DDB004 — encoder/type compatibility

Each encoder accepts specific Go field types:

| encoder | accepted types |
|---|---|
| (none), `upper`, `lower`, `ulid`, `urlenc` | `string` |
| `rfc3339` | `time.Time` |
| `epoch`, `epochms` | `time.Time`, `int64` |
| `pad<N>` | `int64`, unsigned integers |
| `hex` | `[]byte`, `[N]byte` |

## DDB005 — version/ttl typing

`version=` must name an exported, marshaled integer field (`int`, `int32`,
`int64`). `ttl=` must name an `int64` field holding unix seconds, because
that is what DynamoDB's TTL expects. Neither field may appear as a key
template placeholder: the version changes on every update without
recomputing keys (the index would silently drift), and TTL values change
over an item's life.

## DDB006 — duplicate names

Entity `type=` strings must be unique per table (collection dispatch on
the entity-type attribute would otherwise be ambiguous), and pattern names
must be unique per table (generated method names would collide).

## DDB007 — placeholder resolution

Every `{Field}` in every key template must resolve to an exported struct
field that participates in marshaling. Unexported fields and fields tagged
`dynamodbav:"-"` cannot feed keys.

## DDB008 — reserved attribute names

Field attribute names must not collide with the synthesized physical key
attributes (`pk`, `sk`, `gsi1pk`, …) or the entity-type attribute
(`_et` or the `et=` override): marshal injects those after `MarshalMap`,
so a colliding field would be silently overwritten on every write and
corrupted on every read. Two fields of one entity mapping to the same
attribute name are rejected for the same reason.
