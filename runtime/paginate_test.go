package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func sItem(pk, sk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: pk},
		"sk": &types.AttributeValueMemberS{Value: sk},
	}
}

func TestQueryAllPaginatesAndPropagatesErrors(t *testing.T) {
	ctx := context.Background()
	spec := QuerySpec{Table: "t", PKAttr: "pk", PKValue: "P"}

	t.Run("streams across pages", func(t *testing.T) {
		fake := &fakeDB{queryResponses: []queryResponse{
			{out: &dynamodb.QueryOutput{
				Items:            []map[string]types.AttributeValue{sItem("P", "a"), sItem("P", "b")},
				LastEvaluatedKey: sItem("P", "b"),
			}},
			{out: &dynamodb.QueryOutput{
				Items: []map[string]types.AttributeValue{sItem("P", "c")},
			}},
		}}
		var got []string
		for av, err := range QueryAllRaw(ctx, fake, spec) {
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, EntityType(av, "sk"))
		}
		if len(got) != 3 || got[0] != "a" || got[2] != "c" {
			t.Fatalf("items: %v", got)
		}
		if fake.queryCalls != 2 {
			t.Fatalf("expected 2 pages, got %d", fake.queryCalls)
		}
	})

	t.Run("mid-stream error stops iteration", func(t *testing.T) {
		boom := errors.New("boom")
		fake := &fakeDB{queryResponses: []queryResponse{
			{out: &dynamodb.QueryOutput{
				Items:            []map[string]types.AttributeValue{sItem("P", "a")},
				LastEvaluatedKey: sItem("P", "a"),
			}},
			{err: boom},
		}}
		var items, errs int
		for _, err := range QueryAllRaw(ctx, fake, spec) {
			if err != nil {
				errs++
				if !errors.Is(err, boom) {
					t.Fatalf("wrong error: %v", err)
				}
				continue
			}
			items++
		}
		if items != 1 || errs != 1 {
			t.Fatalf("items=%d errs=%d", items, errs)
		}
	})

	t.Run("early break stops paging", func(t *testing.T) {
		fake := &fakeDB{queryResponses: []queryResponse{
			{out: &dynamodb.QueryOutput{
				Items:            []map[string]types.AttributeValue{sItem("P", "a"), sItem("P", "b")},
				LastEvaluatedKey: sItem("P", "b"),
			}},
		}}
		for range QueryAllRaw(ctx, fake, spec) {
			break
		}
		if fake.queryCalls != 1 {
			t.Fatalf("early break must not fetch further pages, got %d calls", fake.queryCalls)
		}
	})

	t.Run("empty condition short-circuits without a request", func(t *testing.T) {
		fake := &fakeDB{}
		emptySpec := spec
		emptySpec.SKAttr = "sk"
		emptySpec.SKCond = SKEmpty()
		for range QueryAllRaw(ctx, fake, emptySpec) {
			t.Fatal("empty condition must yield nothing")
		}
		if fake.queryCalls != 0 {
			t.Fatalf("empty condition must not call DynamoDB, got %d calls", fake.queryCalls)
		}
	})
}

func TestBuildQueryInputFilters(t *testing.T) {
	in := BuildQueryInput(QuerySpec{
		Table: "t", PKAttr: "pk", PKValue: "P",
		ETAttr: "_et", ETValue: "order",
	})
	if got := *in.FilterExpression; got != "#ddbet = :ddbet" {
		t.Fatalf("filter: %q", got)
	}
	if in.ExpressionAttributeNames["#ddbet"] != "_et" {
		t.Fatalf("names: %v", in.ExpressionAttributeNames)
	}

	in = BuildQueryInput(QuerySpec{
		Table: "t", PKAttr: "pk", PKValue: "P",
		ETAttr: "_et", ETValue: "order",
		Filter: &Filter{
			Expression: "#total > :min",
			Names:      map[string]string{"#total": "total"},
			Values:     map[string]types.AttributeValue{":min": &types.AttributeValueMemberN{Value: "5"}},
		},
	})
	if got := *in.FilterExpression; got != "#ddbet = :ddbet AND (#total > :min)" {
		t.Fatalf("combined filter: %q", got)
	}
	if in.ExpressionAttributeNames["#total"] != "total" {
		t.Fatalf("merged names: %v", in.ExpressionAttributeNames)
	}
}
