# ddbgen

**ElectroDB's model, sqlc's ergonomics, Go's type system.**

`ddbgen` reads Go structs annotated with `//ddb:` marker comments describing a
single-table DynamoDB design — key templates, GSIs, access patterns — and
generates a fully typed client, the table's infrastructure definition, and an
access-pattern document, all from the same parse.

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
   entity. Generated code makes each access pattern one typed method, aliases
   every attribute name, and derives range bounds from the key template so a
   query for orders can never return payments — even in shared partitions.

Static checks run at generate time: key collisions between entities,
unsatisfiable patterns, unsortable key segments, encoder/type mismatches —
each with a stable error code ([docs/checks.md](docs/checks.md)) and a
file:line diagnostic.

## Install

```sh
go install github.com/ResonanceCache/ddbgen/cmd/ddbgen@latest
```

Generated code depends on `aws-sdk-go-v2` and the tiny
`github.com/ResonanceCache/ddbgen/runtime` package (no reflection in hot
paths, ~800 lines, fully godoc'd). Go 1.23+ (`iter.Seq2`).

## Quickstart

1. Annotate your structs with `//ddb:` markers (see the model above, or
   [examples/ecommerce/model.go](examples/ecommerce/model.go)).
2. Generate:

   ```sh
   ddbgen generate ./...          # typed client + ddb.snapshot.json
   ddbgen docs ./...              # ACCESS_PATTERNS.md
   ddbgen infra --format cfn ./...  # infra/table_<name>.cfn.yaml (or --format tf)
   ```

3. Wire it up:

   ```go
   ddb := dynamodb.NewFromConfig(cfg)
   app := NewAppClient(ddb, "app")
   ```

4. In CI, `ddbgen diff ./...` fails on breaking schema changes against the
   committed snapshot (changed key templates, removed entities or patterns,
   renamed physical attributes) and allows additive ones.

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
               sk.between | sk.gt | sk.gte | sk.lt | sk.lte]
```

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
- **DDB002** pattern satisfiability (pk identity, boundary-aligned sk conditions)
- **DDB003** sortability of range-condition placeholders
- **DDB004** encoder/type compatibility
- **DDB005** version/ttl field typing
- **DDB006** duplicate entity types or pattern names per table
- **DDB007** placeholder resolution

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

## Roadmap

- CDK (Go) emitter
- configurable delimiter and physical attribute names
- shard-suffix key templates (write sharding)
- LSI support if a compelling case shows up (see FAQ)

Shipped beyond v1 scope: a thin `TransactWrite` passthrough
(`TransactPut<Entity>` / `TransactDelete<Entity>` builders) and a
`ddbgen init` scaffolder.

## License

Apache-2.0
