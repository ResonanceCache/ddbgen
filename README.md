# ddbgen

[![ci](https://github.com/ResonanceCache/ddbgen/actions/workflows/ci.yml/badge.svg)](https://github.com/ResonanceCache/ddbgen/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ResonanceCache/ddbgen.svg)](https://pkg.go.dev/github.com/ResonanceCache/ddbgen/runtime)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**ElectroDB's model, sqlc's ergonomics, Go's type system.**

`ddbgen` reads Go structs annotated with `//ddb:` marker comments describing a
single-table DynamoDB design — key templates, GSIs, access patterns — and
generates a fully typed client, the table's infrastructure definition, and an
access-pattern document, all from the same parse.

> **Writeup:** [Type-safe single-table DynamoDB in Go — and the two silent-wrong-data
> bugs I almost shipped](docs/blog/typed-single-table-dynamodb.md) covers the
> lexicographic-ordering invariant, the exclusive-range-bound construction, and
> what an adversarial audit plus a real migration each caught that the other
> missed.

```go
//ddb:entity table=app type=order version=Ver ttl=ExpiresAt
//ddb:key pk="TENANT#{TenantID}" sk="ORDER#{CreatedAt:rfc3339}#{OrderID}"
//ddb:index name=GSI1 pk="STATUS#{Status:upper}" sk="{UpdatedAt:epoch}"
//ddb:pattern name=OrdersByTenant index=main pk="TENANT#{TenantID}" sk.begins="ORDER#"
//ddb:pattern name=OrdersByStatus index=GSI1 pk="STATUS#{Status:upper}"
type Order struct {
    TenantID  string    `dynamodbav:"tenant_id"`
    OrderID   string    `dynamodbav:"order_id"`
    Status    string    `dynamodbav:"status"`
    Total     int64     `dynamodbav:"total"`
    CreatedAt time.Time `dynamodbav:"created_at"`
    UpdatedAt time.Time `dynamodbav:"updated_at"`
    Ver       int64     `dynamodbav:"v"`
    ExpiresAt int64     `dynamodbav:"exp,omitempty"`
}
```

```go
app := NewAppClient(ddb, "app")

// Typed pattern query with a range cut derived from the key template.
for o, err := range app.OrdersByTenant("acme").CreatedAfter(since).Desc().All(ctx) { … }

// Optimistic locking from the version= marker; ErrVersionConflict on a lost race.
err := app.PutOrder(ctx, order)

// Setting Status recomputes gsi1pk in the same update. The index cannot drift.
updated, err := app.UpdateOrder("acme", createdAt, "o1").SetStatus("shipped").Run(ctx)
```

## Why

Hand-written single-table DynamoDB code fails in two quiet ways:

1. **Synthesized key drift.** Your GSI keys are derived data — `gsi1pk =
   "STATUS#" + strings.ToUpper(o.Status)` — maintained by hand at every write
   site. Miss one `UpdateItem` and the index silently diverges from the data.
   `ddbgen` computes every key attribute from source fields inside the
   generated marshal and update paths; there is no write site to miss.
2. **Stringly-typed everything.** Key conditions, expression names, reserved
   words, cursor plumbing, `begins_with` prefixes that quietly match the wrong
   entity. Generated code makes each access pattern one typed method and
   aliases every attribute name. Range bounds derived from the key template
   keep the scanned range tight, and a server-side entity-type filter on every
   pattern query makes the typed result exact: a query for orders cannot
   return payments, even in shared partitions or hierarchical key designs.

Static checks run at generate time: key collisions between entities,
unsatisfiable patterns, unsortable key segments, encoder/type mismatches,
reserved-attribute shadowing — each with a stable error code
([docs/checks.md](docs/checks.md)) and a file:line diagnostic.

## Install

```sh
go install github.com/ResonanceCache/ddbgen/cmd/ddbgen@latest
```

On Go 1.24+, pin it as a module tool instead (recorded in go.mod, versioned
with your repo):

```sh
go get -tool github.com/ResonanceCache/ddbgen/cmd/ddbgen@latest
go tool ddbgen generate ./...
```

Or invoke it through `go generate` (the scaffold from `ddbgen init`
includes this):

```go
//go:generate go run github.com/ResonanceCache/ddbgen/cmd/ddbgen generate .
```

Generated code depends on `aws-sdk-go-v2` and the
`github.com/ResonanceCache/ddbgen/runtime` package (~1k lines, fully
godoc'd; key encoding and expression assembly are reflection-free — item
(un)marshaling uses the AWS `attributevalue` package, like hand-written
SDK code would). Go 1.23+ (`iter.Seq2`).

## Quickstart

1. Annotate your structs with `//ddb:` markers (see the model above, or
   [examples/ecommerce/model.go](examples/ecommerce/model.go)). All
   entities of one table live in one package.
2. Generate, then fetch the generated code's dependencies:

   ```sh
   ddbgen generate ./...            # typed client + ddb.snapshot.json
   ddbgen docs ./...                # ACCESS_PATTERNS.md
   ddbgen infra --format cfn ./...  # infra/table_<name>.cfn.yaml (or --format tf)
   go mod tidy
   ```

3. Wire it up:

   ```go
   ddb := dynamodb.NewFromConfig(cfg)
   app := NewAppClient(ddb, "app")
   ```

4. In CI, `ddbgen diff ./...` fails (exit 1) on breaking schema changes
   against the committed snapshot (changed key templates, removed entities
   or patterns, renamed physical attributes, changed field types) and
   allows additive ones.

Package patterns resolve like `go vet`, relative to the current module —
in a monorepo with nested modules, run ddbgen once per module.

## Testing your code

`NewAppClient` accepts the `runtime.DynamoDB` interface — the eight
operations generated code uses — rather than a concrete `*dynamodb.Client`,
following the AWS SDK for Go v2 testing guidance. Substitute a mock (or a
wrapping middleware) in tests; see
[testdata/codegen/bounds/harness_test.go](testdata/codegen/bounds/harness_test.go)
for a complete in-memory fake that ddbgen's own test suite runs generated
queries against. `client.DynamoDB()` and `client.TableName()` expose the
underlying handle for raw operations the generated surface does not cover.

Try the runnable example — five minutes, Docker required:

```sh
git clone https://github.com/ResonanceCache/ddbgen && cd ddbgen
make demo          # starts DynamoDB Local, creates a table, runs every method category
```

## Marker reference

```
//ddb:entity  table=<ident> type=<ident> [version=<Field>] [ttl=<Field>] [et=<attr>]
//ddb:key     pk="<template>" [sk="<template>"]
//ddb:index   name=<ident> pk="<template>" [sk="<template>"] [projection=all|keys_only]
//ddb:pattern name=<Ident> index=main|<indexname> pk="<template>"
              [sk.eq="<template>" | sk.begins="<prefix>" |
               sk.gt="<prefix>" | sk.gte="<prefix>" | sk.lt="<prefix>" | sk.lte="<prefix>" |
               sk.between | sk.gt | sk.gte | sk.lt | sk.lte]
```

Valued range conditions (`sk.gt="ORDER#{CreatedAt:rfc3339}"`) fix the bound
in the marker; the bare flags declare intent and take their bounds at call
time through the generated `<Field>After/Before/Between` methods.

| marker | what it declares |
|---|---|
| `entity` | table membership, the entity-type discriminator value (`type=`), optional optimistic-locking field (`version=`), TTL field (`ttl=`), and discriminator attribute override (`et=`, default `_et`) |
| `key` | main-index key templates; physical attributes are `pk`/`sk` |
| `index` | a GSI's key templates; physical attributes derive from the name (`GSI1` → `gsi1pk`/`gsi1sk`) |
| `pattern` | one named access pattern → one generated query method; `sk.*` picks the static sort-key condition, bare range markers (`sk.between`, `sk.gt`, …) document intent and range through generated `<Field>After/Before/Between` methods |

Key templates are `#`-delimited sequences of literals and `{Field[:encoder]}`
placeholders. Values containing the delimiter are rejected at runtime
(`runtime.ErrDelimiterInValue`); `urlenc` is the escape hatch.

## Encoders

| encoder | Go types | encoding | fixed-width |
|---|---|---|---|
| (none) | `string` | raw, delimiter-checked | no |
| `rfc3339` | `time.Time` | `2006-01-02T15:04:05.000000000Z` — forced UTC, 9-digit nanos | 30 |
| `epoch` | `time.Time`, `int64` | seconds, zero-padded; rejects negatives | 12 |
| `epochms` | `time.Time`, `int64` | milliseconds, zero-padded; rejects negatives | 15 |
| `pad<N>` | `int64` ≥ 0, unsigned ints | zero-padded to N; errors on overflow | N |
| `upper` / `lower` | `string` | case-normalized | no |
| `hex` | `[]byte`, `[N]byte` | lowercase hex | 2N for `[N]byte` |
| `ulid` | `string` | validated 26-char Crockford, uppercased (no dependency) | 26 |
| `urlenc` | `string` | `url.QueryEscape` — the delimiter escape hatch | no |

Fixed-width encoders are what make range cuts legal: lexicographic order of
the encoding matches semantic order of the value (property-tested with 1k
random pairs per encoder).

## Static checks

Every `generate`/`diff` run enforces ([full docs](docs/checks.md)):

- **DDB001** key collision/ambiguity between entities (conservative)
- **DDB002** pattern satisfiability (pk identity, boundary-aligned sk conditions, no keys_only patterns)
- **DDB003** sortability of range-condition placeholders
- **DDB004** encoder/type compatibility
- **DDB005** version/ttl field typing (and neither may feed a key template)
- **DDB006** duplicate entity types or pattern names per table
- **DDB007** placeholder resolution
- **DDB008** reserved attribute names (fields may not shadow `pk`/`sk`/GSI keys/`_et`)

Beyond the coded checks, generation fails on colliding generated
identifiers or file names, and key segments that encode to the empty
string fail loudly at runtime (`runtime.ErrEmptySegment`) instead of
writing junk index entries.

## Query surface notes

- Every pattern query carries a server-side `entity-type = <type>` filter;
  key bounds keep the scanned range tight, the filter makes the typed
  result exact. If other tools write items without the entity-type
  attribute into the same table, those items are invisible to pattern
  queries (partition `Collect()` surfaces them under `Unknown`).
- `Filter(expr, names, values)` is the raw escape hatch for non-key
  predicates; it composes with the entity filter. Filtered items still
  consume read capacity.
- `Limit(n)` is DynamoDB's per-page evaluated-items cap, not a result
  cap: `All` still streams every page.
- Reads: `runtime.WithConsistentRead()` on `Get*` and `BatchGet*` (main
  index only). Deletes: `runtime.WithMustExist()` /
  `runtime.WithExpectVersion(v)`.
- Batch: `BatchGet*` dedupes keys (DynamoDB rejects duplicates wholesale);
  `BatchPut*` returns `runtime.ErrDuplicateKey` for two writes to one key;
  exhausted retries return a `*runtime.UnprocessedError` carrying the
  leftovers.
- `TransactPut*`/`TransactDelete*` build items for `TransactWrite` (at
  most 100, atomic). Condition failures inside the transaction match both
  `runtime.ErrConditionFailed` and the SDK's
  `TransactionCanceledException` (with `CancellationReasons`) via
  `errors.Is`/`errors.As`.

## Compared to

| | raw SDK v2 | guregu/dynamo | ddbgen |
|---|---|---|---|
| Typed per-pattern query methods | — | — | ✔ |
| Single-table key templates | hand-rolled | hand-rolled | declared once, compiled |
| Synthesized GSI attributes kept in sync | manual at every write | manual at every write | automatic in marshal + update |
| Item collections (multi-entity partitions) | manual unmarshal switch | manual | typed `Collect()` |
| Generate-time schema checks | — | — | DDB001–DDB007 |
| Infra emitted from the same schema | — | — | CloudFormation + Terraform |
| Optimistic locking | hand-written conditions | partial | `version=` marker |
| Runtime reflection in hot paths | attributevalue only | struct reflection | attributevalue only |

## Access-pattern doc

`ddbgen docs` regenerates `ACCESS_PATTERNS.md` from the same parse as the
code — the single-table design doc that is always current:

```markdown
| Pattern         | Index | Key condition                                              | Returns                       | Generated method                    |
|-----------------|-------|------------------------------------------------------------|-------------------------------|-------------------------------------|
| OrdersByStatus  | GSI1  | pk = "STATUS#{Status:upper}" — refinable via UpdatedAfter…  | []Order (All iterator / Page) | `OrdersByStatus(status)`            |
| OrdersByTenant  | main  | pk = "TENANT#{TenantID}" AND begins_with(sk, "ORDER#") …    | []Order (All iterator / Page) | `OrdersByTenant(tenantID)`          |
| TenantPartition | main  | pk = "TENANT#{TenantID}"                                    | TenantCollection{…}           | `TenantPartition(tenantID).Collect` |
```

## FAQ

**Why marker comments instead of struct tags?** Key templates span multiple
fields — `sk="ORDER#{CreatedAt:rfc3339}#{OrderID}"` belongs to the struct,
not to any one field. Tags are the wrong shape; `dynamodbav` tags keep doing
what they already do (attribute names and marshaling).

**Why no LSIs?** LSIs must be declared at table creation, cap partitions at
10 GB, and almost everything an LSI does a GSI does with fewer regrets. v1 is
GSI-only.

**What about PartiQL?** No. PartiQL hides the difference between a Query and
a Scan, which is the difference between a design and an outage. ddbgen exists
to make key-based access patterns explicit.

**Multiple tables?** v1 compiles each `table=` group independently — one
client per table already works. Cross-table niceties may come later.

**Migrations?** Out of scope. The snapshot diff tells you *that* a change is
breaking; deciding how to migrate stored keys is a human decision.

**Why no config file?** Everything ddbgen needs lives in the markers, next
to the structs they describe. A config file would be a second place for
schema truth to drift.

**Sparse GSIs?** Not yet. A zero-valued index source fails the write
loudly (`runtime.ErrEmptySegment`) rather than silently indexing junk;
conditional index membership is on the roadmap.

## Roadmap

- sparse GSIs (conditional index membership)
- projection expressions and count-only queries
- streaming/paged item-collection queries (today `Collect()` drains)
- `TransactGet` builders and transactional condition checks
- CDK (Go) emitter
- configurable delimiter and physical attribute names
- shard-suffix key templates (write sharding)
- LSI support if a compelling case shows up (see FAQ)

Shipped beyond the original v1 scope: a thin `TransactWrite` passthrough
(`TransactPut<Entity>` / `TransactDelete<Entity>` builders), a
`ddbgen init` scaffolder, interface-based client injection, raw filter
expressions, consistent reads, and conditional deletes.

## License

Apache-2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).
