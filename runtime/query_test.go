package runtime

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestCursorRoundTrip(t *testing.T) {
	lek := map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: "TENANT#t1"},
		"sk": &types.AttributeValueMemberS{Value: "ORDER#x"},
	}
	c, err := EncodeCursor(lek)
	if err != nil {
		t.Fatal(err)
	}
	if c == "" {
		t.Fatal("non-empty LEK must produce non-empty cursor")
	}
	back, err := c.Decode()
	if err != nil {
		t.Fatal(err)
	}
	if got := back["pk"].(*types.AttributeValueMemberS).Value; got != "TENANT#t1" {
		t.Fatalf("pk = %q", got)
	}
	if c, err := EncodeCursor(nil); err != nil || c != "" {
		t.Fatalf("empty LEK: %q, %v", c, err)
	}
	if esk, err := Cursor("").Decode(); err != nil || esk != nil {
		t.Fatalf("empty cursor: %v, %v", esk, err)
	}
	if _, err := Cursor("!!!").Decode(); err == nil {
		t.Fatal("garbage cursor must error")
	}
	if _, err := EncodeCursor(map[string]types.AttributeValue{
		"n": &types.AttributeValueMemberN{Value: "1"},
	}); err == nil {
		t.Fatal("non-string key attribute must error")
	}
}

func TestSKConditions(t *testing.T) {
	// Shared partition (scope non-empty): everything is two-sided.
	after := SKAfter("ORDER#", "ORDER#0002")
	if after.Kind != "between" || after.Lo != "ORDER#0002"+MaxKeySuffix || after.Hi != "ORDER#"+MaxKeySuffix {
		t.Fatalf("SKAfter: %+v", after)
	}
	before := SKBefore("ORDER#", "ORDER#0002", "ORDER#0001", true)
	if before.Kind != "between" || before.Lo != "ORDER#" || before.Hi != "ORDER#0001"+MaxKeySuffix {
		t.Fatalf("SKBefore: %+v", before)
	}
	if c := SKBefore("ORDER#", "ORDER#0000", "", false); c.Kind != "empty" {
		t.Fatalf("SKBefore underflow: %+v", c)
	}
	if c := SKOnOrAfter("ORDER#", "ORDER#0002"); c.Kind != "between" || c.Lo != "ORDER#0002" || c.Hi != "ORDER#"+MaxKeySuffix {
		t.Fatalf("SKOnOrAfter: %+v", c)
	}
	if c := SKOnOrBefore("ORDER#", "ORDER#0002"); c.Kind != "between" || c.Lo != "ORDER#" || c.Hi != "ORDER#0002"+MaxKeySuffix {
		t.Fatalf("SKOnOrBefore: %+v", c)
	}
	// Exclusive partitions (scope empty) use one-sided conditions.
	if c := SKAfter("", "0002"); c.Kind != "gt" || c.Lo != "0002"+MaxKeySuffix {
		t.Fatalf("SKAfter no scope: %+v", c)
	}
	if c := SKBefore("", "0002", "0001", true); c.Kind != "lt" || c.Lo != "0002" {
		t.Fatalf("SKBefore no scope: %+v", c)
	}
	if c := SKOnOrAfter("", "0002"); c.Kind != "gte" || c.Lo != "0002" {
		t.Fatalf("SKOnOrAfter no scope: %+v", c)
	}
	if c := SKOnOrBefore("", "0002"); c.Kind != "lte" || c.Lo != "0002"+MaxKeySuffix {
		t.Fatalf("SKOnOrBefore no scope: %+v", c)
	}
	if c := SKEq("ORDER#x"); c.Kind != "eq" || c.Lo != "ORDER#x" {
		t.Fatalf("SKEq: %+v", c)
	}
	if c := SKBegins("ORDER#"); c.Kind != "begins" || c.Lo != "ORDER#" {
		t.Fatalf("SKBegins: %+v", c)
	}
	if c := SKBetween("A#1", "A#2"); c.Kind != "between" || c.Hi != "A#2"+MaxKeySuffix {
		t.Fatalf("SKBetween: %+v", c)
	}
	// Degenerate inputs are provably empty, never invalid BETWEENs.
	if c := SKBetween("A#2", "A#1"); c.Kind != "empty" {
		t.Fatalf("SKBetween reversed: %+v", c)
	}
	if c := SKAfter("ORDER#", "OR"); c.Kind != "empty" {
		t.Fatalf("SKAfter degenerate: %+v", c)
	}
	if c := SKOnOrAfter("ORDER#", "ORDER#"+MaxKeySuffix+"x"); c.Kind != "empty" {
		t.Fatalf("SKOnOrAfter above scope: %+v", c)
	}
}

func TestBuildQueryInput(t *testing.T) {
	in := BuildQueryInput(QuerySpec{
		Table:   "app",
		Index:   "GSI1",
		PKAttr:  "gsi1pk",
		PKValue: "STATUS#OPEN",
		SKAttr:  "gsi1sk",
		SKCond:  SKBetween("000000000001", "000000000005"),
		Desc:    true,
		Limit:   10,
	})
	if got := *in.KeyConditionExpression; got != "#pk = :pk AND #sk BETWEEN :a AND :b" {
		t.Fatalf("expr: %q", got)
	}
	if in.ExpressionAttributeNames["#sk"] != "gsi1sk" || *in.IndexName != "GSI1" {
		t.Fatalf("names: %+v index %v", in.ExpressionAttributeNames, in.IndexName)
	}
	if *in.ScanIndexForward || *in.Limit != 10 {
		t.Fatalf("flags: forward=%v limit=%v", *in.ScanIndexForward, *in.Limit)
	}
	plain := BuildQueryInput(QuerySpec{Table: "app", PKAttr: "pk", PKValue: "X"})
	if got := *plain.KeyConditionExpression; got != "#pk = :pk" {
		t.Fatalf("plain expr: %q", got)
	}
	if plain.IndexName != nil || plain.Limit != nil || plain.ConsistentRead != nil {
		t.Fatal("plain query must not set optional fields")
	}
}

func TestUpdateExpr(t *testing.T) {
	var e UpdateExpr
	if !e.Empty() {
		t.Fatal("fresh expr must be empty")
	}
	e.Set("status", &types.AttributeValueMemberS{Value: "OPEN"})
	e.Add("total", &types.AttributeValueMemberN{Value: "5"})
	e.Remove("exp")
	e.IncrementVersion("v")
	got := e.Expression()
	want := "SET #n0 = :u1, #n5 = if_not_exists(#n5, :u6) + :u7 REMOVE #n4 ADD #n2 :u3"
	if got != want {
		t.Fatalf("expression:\n got %q\nwant %q", got, want)
	}
	names := e.Names(map[string]string{"#p0": "pk"})
	if names["#n0"] != "status" || names["#p0"] != "pk" || names["#n5"] != "v" {
		t.Fatalf("names: %+v", names)
	}
	values := e.Values(nil)
	if values[":u6"].(*types.AttributeValueMemberN).Value != "0" {
		t.Fatalf("values: %+v", values)
	}
}
