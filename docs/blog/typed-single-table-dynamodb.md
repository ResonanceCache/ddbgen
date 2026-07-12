# Type-safe single-table DynamoDB in Go — and the two silent-wrong-data bugs I almost shipped

DynamoDB's single-table pattern is powerful and error-prone in the same breath. You put every entity — orders, payments, users — in one table, overload the keys, and lean on secondary indexes to serve each access pattern. Done well it's fast and cheap. Done in hand-written Go it drifts and lies, quietly, in ways your tests don't catch.

I built [`ddbgen`](https://github.com/ResonanceCache/ddbgen), a code generator that turns annotated Go structs into a typed single-table client, to make those failure modes unrepresentable. This post is about the one idea that makes the whole thing work — lexicographic ordering — a trick for range queries DynamoDB doesn't natively support, and the part I'm least proud of and most glad I did: an adversarial audit and a real-world migration that each caught a bug the other missed.

## The two ways hand-written single-table code lies

**Synthesized-key drift.** Your GSI partition key isn't data you store once; it's *derived* data you have to recompute at every write site:

```go
item["gsi1pk"] = &types.AttributeValueMemberS{Value: "STATUS#" + strings.ToUpper(order.Status)}
```

Miss one `UpdateItem` — a status change that forgets to rewrite `gsi1pk` — and your index silently disagrees with your table. No error. Queries just start returning stale or missing rows.

**Stringly-typed everything.** Key conditions, expression-attribute aliases, reserved-word escaping, `begins_with` prefixes, cursor plumbing. All strings, all hand-assembled, none checked until runtime — and some not even then. A `begins_with` prefix meant to scope to one entity happily matches another that shares the partition.

`ddbgen` closes both. You write the schema once, as marker comments:

```go
//ddb:entity table=app type=order version=Ver ttl=ExpiresAt
//ddb:key pk="TENANT#{TenantID}" sk="ORDER#{CreatedAt:rfc3339}#{OrderID}"
//ddb:index name=GSI1 pk="STATUS#{Status:upper}" sk="{UpdatedAt:epoch}"
//ddb:pattern name=OrdersByTenant index=main pk="TENANT#{TenantID}" sk.begins="ORDER#"
type Order struct {
    TenantID  string    `dynamodbav:"tenant_id"`
    OrderID   string    `dynamodbav:"order_id"`
    Status    string    `dynamodbav:"status"`
    CreatedAt time.Time `dynamodbav:"created_at"`
    UpdatedAt time.Time `dynamodbav:"updated_at"`
    Ver       int64     `dynamodbav:"v"`
    ExpiresAt int64     `dynamodbav:"exp,omitempty"`
}
```

and get a typed client where every access pattern is one method:

```go
for o, err := range app.OrdersByTenant("acme").CreatedAfter(since).Desc().All(ctx) { ... }
```

The GSI key is recomputed inside the generated marshal and update paths — there is no write site to forget. That's the drift fix, and it's mostly plumbing. The interesting part is `CreatedAfter`.

## The one idea: sort keys are strings, so order is everything

DynamoDB sort keys are byte strings. A range query — "orders created after T" — is a lexicographic comparison on those bytes. This works if and only if the *string* order of your encoding matches the *semantic* order of your values.

For strings that's free. For anything else it's a trap:

- `"9" > "10"` lexicographically. Store an unpadded integer counter and your range queries are quietly wrong.
- `"2026-1-9" > "2026-01-10"` — a non-zero-padded date sorts after a later one.
- `time.Time.String()` includes a monotonic-clock suffix and a timezone; two encodings of the same instant don't even compare equal.

So `ddbgen` ships a fixed set of encoders whose lexicographic order provably matches value order, and won't let you range over anything else:

| encoder | value | encoding |
|---|---|---|
| `rfc3339` | `time.Time` | `2006-01-02T15:04:05.000000000Z` — forced UTC, 9 fixed nanos |
| `epoch` / `epochms` | time or int | zero-padded seconds / millis |
| `pad<N>` | integer | zero-padded to width N |
| `ulid` | string | validated 26-char Crockford |

Every one is **fixed-width**. That's the requirement: if `CreatedAt` weren't a fixed 30 characters, `"…04:59:59Z#zzz"` could sort after `"…05:00:00Z#a"` and a range cut would slice in the wrong place. A static check (one of eight, `DDB001`–`DDB008`) rejects at *generate* time any range condition over a variable-width segment, with a file:line diagnostic:

```
DDB003: pattern OrdersAfter: sk.gt ranges over placeholder {Name}, whose
encoding is variable-width; lexicographic comparison would not follow value
order — use a fixed-width encoder
```

The property is worth testing hard, so the encoder suite generates 1,000 random value pairs per encoder and asserts that string order matches value order every time. It's the load-bearing invariant; everything below assumes it.

## The trick: exclusive bounds without an exclusive operator

Here's a thing DynamoDB doesn't give you: an exclusive range on a *prefix* of a compound key.

Take `sk = "ORDER#{CreatedAt:rfc3339}#{OrderID}"`. You want "orders strictly after time T," regardless of `OrderID`. You can't write `sk > "ORDER#<T>"` — that would also skip every order *at* exactly T with a small `OrderID`, and include unrelated rows. `begins_with` doesn't take a range. And `BETWEEN` is *inclusive* on both ends.

The construction that works uses a max-suffix sentinel. `ddbgen` defines:

```go
const MaxKeySuffix = "￿" // sorts after every byte the fixed-width encoders emit
```

"Strictly after T" within a shared partition becomes a two-sided `BETWEEN`:

```
BETWEEN  "ORDER#" + enc(T) + "￿"   AND   "ORDER#" + "￿"
```

The lower bound sits just past every key whose time is exactly T (because `￿` sorts after any real `OrderID`); the upper bound stops at the end of the `ORDER#` scope, so the query can't spill into a sibling entity like `PAY#...` that shares the partition and happens to sort adjacent.

"Strictly *before* T" is the mirror image, and it needs one more piece: the encoding of T's predecessor. There's a `Pred*` function per encoder that returns the largest encodable value strictly below its input — `PredRFC3339(t)` is `t` minus one nanosecond, re-encoded; `PredPad(n, w)` is `n-1` zero-padded. The bound becomes `BETWEEN scope AND pred(T) + "￿"`. If the predecessor underflows (nothing sorts below T), the whole condition collapses to a statically-empty result that short-circuits without a network call.

This is the kind of thing that's a paragraph to describe, a day to get exactly right, and a single off-by-one away from returning the wrong rows forever. Which brings me to the part I actually want to write about.

## Then I tried to break it

When the tool "worked" — example app green, integration tests passing against DynamoDB Local — I didn't trust it. Passing tests prove the cases you thought of. Range-bound math fails on the cases you didn't.

So I ran an adversarial audit: fan out reviewers across every subsystem, have each one hunt for a *concrete* failure scenario, then have a second pass try to *refute* each finding before it counted. Fifty-four review passes. It surfaced two confirmed bugs that every existing test was blind to — both in the range-bound code, both returning wrong data with no error.

**Bug one: the missing delimiter.** A pattern written `sk.begins="ORDER"` — no trailing `#` — validated fine but generated bounds against the literal `"ORDER"`, while real keys are `ORDER#...`. Because `#` (0x23) sorts *below* every character the encoders emit (digits and letters, ≥ 0x30), `CreatedAfter` matched *nothing* and `CreatedBefore` matched *everything*, regardless of the argument. Silent. The fix was to canonicalize any begins-prefix that stops short of the full key to always carry the delimiter.

**Bug two: the entity leak.** I'd convinced myself that tight key bounds alone kept a query for orders from returning payments. The audit produced a legal, check-passing schema where they didn't — an entity whose sort key starts with a placeholder, sharing a partition with a sibling of a different shape. The range bound scooped the sibling's rows and unmarshaled them into the wrong struct as zero-valued fields. The fix: every pattern query now also carries a server-side `entity-type = <type>` filter. Bounds do the performance work; the filter makes the typed result *exact*.

The lesson wasn't "write more tests." It was **verify against a model, not against examples.** I built a harness that compiles the *generated* client for a deliberately leak-prone schema and runs its queries against an in-memory DynamoDB that implements real lexicographic semantics — then asserts the returned set equals the set a plain string-comparison filter would return. I reverted each fix and watched the harness fail exactly where the audit said it would. That harness is worth more than the fixes; it's what would catch the *next* range-bound bug.

## Dogfooding caught my own fix

An audit finds bugs. Using the thing finds the bugs the audit *introduced*.

I migrated a real project's hand-written DynamoDB layer — five entities, a GSI, a discriminator, TTL, bidirectional writes, scan-based admin paths — onto the generated client. Two things broke immediately, and both were mine:

**I'd over-corrected.** One audit fix had made the validator reject a condition that ends inside a variable-width placeholder. But `sk.eq="{Category}"` on a GSI sort key that *is* a single `{Category}` — an exact match on the whole attribute — is completely fine; the key genuinely ends there. My stricter rule was a false positive that blocked a legitimate, common pattern. The audit had removed a guard it shouldn't have. Real usage found it in the first thirty seconds; no test had, because no test used that shape.

**My escape hatch couldn't reach the one operation you escape for.** The generated client takes an interface (so you can mock it) and exposes `.DynamoDB()` for operations the typed surface doesn't cover. I'd scoped that interface to exactly the eight calls generated code makes — and left out `Scan`, which is *precisely* what you reach the escape hatch to do: admin counts, migrations, full-table audits. The real migration needed it in four places and couldn't get it.

Both became a point release. Neither was findable from inside the project; both were obvious from outside it. That's the entire argument for dogfooding in one paragraph.

## What generalizes

Most of this isn't about DynamoDB or Go:

- **Encode for the query engine's ordering, not for human readability.** The moment a comparison happens on your serialized form, lexicographic order *is* your data model. Fixed-width or bust.
- **Inclusive-only range operators still let you do exclusive ranges** — with a max-suffix sentinel and a predecessor function. Get the predecessor's underflow case right or you'll return a phantom row at the boundary.
- **Passing tests validate your imagination.** For anything where the failure is "wrong result, no error," verify the output against an independent model of what correct means, and prove your test fails when the code is wrong.
- **An audit and dogfooding find disjoint bug sets.** The audit found silent-wrong-data in code I trusted. Dogfooding found a false positive the audit *created* and an API gap no amount of review would have surfaced. You want both.

The generator is [on GitHub](https://github.com/ResonanceCache/ddbgen) — `go install`, `ddbgen init`, and there's a runnable example that stands up DynamoDB Local and exercises every generated method. But the code isn't really the point of this post. The point is that "it builds and the tests pass" was the *start* of the correctness work, not the end of it.
