package runtime

import (
	"context"
	"fmt"
	"iter"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// SKCond is a resolved sort-key condition. Constructors below encode the
// range-cut semantics documented in the generator: in shared partitions
// every range is two-sided (BETWEEN) so items of foreign entities that sort
// adjacent to the scope prefix can never leak into results.
type SKCond struct {
	Kind string // "", "eq", "begins", "lt", "lte", "gt", "gte", "between"
	Lo   string
	Hi   string
}

// SKEq matches sk exactly.
func SKEq(v string) SKCond { return SKCond{Kind: "eq", Lo: v} }

// SKBegins matches sk by prefix.
func SKBegins(prefix string) SKCond { return SKCond{Kind: "begins", Lo: prefix} }

// SKAfter matches keys strictly after the encoded value and all of its
// continuations. scope is the entity's leading-literal sk prefix ("" when
// the entity provably owns the partition alone).
func SKAfter(scope, v string) SKCond {
	lo := v + MaxKeySuffix
	if scope == "" {
		return SKCond{Kind: "gt", Lo: lo}
	}
	hi := scope + MaxKeySuffix
	if lo > hi {
		return SKCond{Kind: "between", Lo: scope, Hi: scope} // provably empty
	}
	return SKCond{Kind: "between", Lo: lo, Hi: hi}
}

// SKOnOrAfter matches the encoded value, its continuations, and everything
// after them.
func SKOnOrAfter(scope, v string) SKCond {
	if scope == "" {
		return SKCond{Kind: "gte", Lo: v}
	}
	hi := scope + MaxKeySuffix
	if v > hi {
		return SKCond{Kind: "between", Lo: scope, Hi: scope}
	}
	return SKCond{Kind: "between", Lo: v, Hi: hi}
}

// SKBefore matches keys strictly before the encoded value. pred is the
// encoding of the value's predecessor (see the Pred* functions); predOK
// false means nothing sorts below the value and the range is provably
// empty within the scope.
func SKBefore(scope, v, pred string, predOK bool) SKCond {
	if scope == "" {
		return SKCond{Kind: "lt", Lo: v}
	}
	if !predOK {
		return SKCond{Kind: "between", Lo: scope, Hi: scope}
	}
	return SKCond{Kind: "between", Lo: scope, Hi: pred + MaxKeySuffix}
}

// SKOnOrBefore matches the encoded value, its continuations, and
// everything before them.
func SKOnOrBefore(scope, v string) SKCond {
	if scope == "" {
		return SKCond{Kind: "lte", Lo: v + MaxKeySuffix}
	}
	return SKCond{Kind: "between", Lo: scope, Hi: v + MaxKeySuffix}
}

// SKBetween matches keys from lo through hi inclusive, including hi's
// continuations.
func SKBetween(lo, hi string) SKCond {
	return SKCond{Kind: "between", Lo: lo, Hi: hi + MaxKeySuffix}
}

// QuerySpec describes one generated query.
type QuerySpec struct {
	Table      string
	Index      string // "" for the main index
	PKAttr     string
	PKValue    string
	SKAttr     string
	SKCond     SKCond
	Desc       bool
	Limit      int32
	Consistent bool
}

// BuildQueryInput assembles the QueryInput with fully aliased attribute
// names and value placeholders.
func BuildQueryInput(s QuerySpec) *dynamodb.QueryInput {
	names := map[string]string{"#pk": s.PKAttr}
	values := map[string]types.AttributeValue{
		":pk": &types.AttributeValueMemberS{Value: s.PKValue},
	}
	cond := "#pk = :pk"
	if s.SKCond.Kind != "" {
		names["#sk"] = s.SKAttr
		values[":a"] = &types.AttributeValueMemberS{Value: s.SKCond.Lo}
		switch s.SKCond.Kind {
		case "eq":
			cond += " AND #sk = :a"
		case "begins":
			cond += " AND begins_with(#sk, :a)"
		case "lt":
			cond += " AND #sk < :a"
		case "lte":
			cond += " AND #sk <= :a"
		case "gt":
			cond += " AND #sk > :a"
		case "gte":
			cond += " AND #sk >= :a"
		case "between":
			values[":b"] = &types.AttributeValueMemberS{Value: s.SKCond.Hi}
			cond += " AND #sk BETWEEN :a AND :b"
		}
	}
	input := &dynamodb.QueryInput{
		TableName:                 aws.String(s.Table),
		KeyConditionExpression:    aws.String(cond),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
		ScanIndexForward:          aws.Bool(!s.Desc),
	}
	if s.Index != "" {
		input.IndexName = aws.String(s.Index)
	}
	if s.Limit > 0 {
		input.Limit = aws.Int32(s.Limit)
	}
	if s.Consistent {
		input.ConsistentRead = aws.Bool(true)
	}
	return input
}

// QueryPageRaw runs a single page query from the given cursor.
func QueryPageRaw(ctx context.Context, ddb *dynamodb.Client, s QuerySpec, cursor Cursor) ([]map[string]types.AttributeValue, Cursor, error) {
	input := BuildQueryInput(s)
	esk, err := cursor.Decode()
	if err != nil {
		return nil, "", err
	}
	input.ExclusiveStartKey = esk
	out, err := ddb.Query(ctx, input)
	if err != nil {
		return nil, "", fmt.Errorf("ddbgen: query: %w", err)
	}
	next, err := EncodeCursor(out.LastEvaluatedKey)
	if err != nil {
		return nil, "", err
	}
	return out.Items, next, nil
}

// QueryPage runs a single page query and unmarshals the items.
func QueryPage[T any](ctx context.Context, ddb *dynamodb.Client, s QuerySpec, cursor Cursor, unmarshal func(map[string]types.AttributeValue) (*T, error)) ([]T, Cursor, error) {
	raw, next, err := QueryPageRaw(ctx, ddb, s, cursor)
	if err != nil {
		return nil, "", err
	}
	items := make([]T, 0, len(raw))
	for _, av := range raw {
		it, err := unmarshal(av)
		if err != nil {
			return nil, "", err
		}
		items = append(items, *it)
	}
	return items, next, nil
}

// QueryAllRaw streams every matching item, paginating internally.
// Iteration stops after yielding the first error.
func QueryAllRaw(ctx context.Context, ddb *dynamodb.Client, s QuerySpec) iter.Seq2[map[string]types.AttributeValue, error] {
	return func(yield func(map[string]types.AttributeValue, error) bool) {
		var cursor Cursor
		for {
			items, next, err := QueryPageRaw(ctx, ddb, s, cursor)
			if err != nil {
				yield(nil, err)
				return
			}
			for _, av := range items {
				if !yield(av, nil) {
					return
				}
			}
			if next == "" {
				return
			}
			cursor = next
		}
	}
}

// QueryAll streams every matching item as a typed value.
func QueryAll[T any](ctx context.Context, ddb *dynamodb.Client, s QuerySpec, unmarshal func(map[string]types.AttributeValue) (*T, error)) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		for av, err := range QueryAllRaw(ctx, ddb, s) {
			if err != nil {
				yield(zero, err)
				return
			}
			it, err := unmarshal(av)
			if err != nil {
				yield(zero, err)
				return
			}
			if !yield(*it, nil) {
				return
			}
		}
	}
}

// EntityType reads the entity-type discriminator attribute from a raw item.
func EntityType(av map[string]types.AttributeValue, attr string) string {
	if s, ok := av[attr].(*types.AttributeValueMemberS); ok {
		return s.Value
	}
	return ""
}
