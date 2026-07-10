package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func TestBatchGetDedupesAndRetries(t *testing.T) {
	ctx := context.Background()
	keys := []map[string]types.AttributeValue{
		sItem("P", "a"), sItem("P", "b"), sItem("P", "a"), // duplicate
	}
	call := 0
	fake := &fakeDB{batchGetFn: func(in *dynamodb.BatchGetItemInput) (*dynamodb.BatchGetItemOutput, error) {
		call++
		req := in.RequestItems["t"]
		if call == 1 {
			if len(req.Keys) != 2 {
				t.Fatalf("duplicates not removed: %d keys", len(req.Keys))
			}
			// Return one item, leave one unprocessed to force a retry.
			return &dynamodb.BatchGetItemOutput{
				Responses: map[string][]map[string]types.AttributeValue{
					"t": {sItem("P", "a")},
				},
				UnprocessedKeys: map[string]types.KeysAndAttributes{
					"t": {Keys: []map[string]types.AttributeValue{sItem("P", "b")}},
				},
			}, nil
		}
		if len(req.Keys) != 1 {
			t.Fatalf("retry must resend only unprocessed keys, got %d", len(req.Keys))
		}
		return &dynamodb.BatchGetItemOutput{
			Responses: map[string][]map[string]types.AttributeValue{
				"t": {sItem("P", "b")},
			},
		}, nil
	}}
	got, err := BatchGet(ctx, fake, "t", keys, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || call != 2 {
		t.Fatalf("items=%d calls=%d", len(got), call)
	}
	if cr := fake.batchGetIn[0].RequestItems["t"].ConsistentRead; cr == nil || !*cr {
		t.Fatal("consistent read not forwarded")
	}
}

func TestBatchGetExhaustedRetriesReturnsLeftovers(t *testing.T) {
	fake := &fakeDB{batchGetFn: func(in *dynamodb.BatchGetItemInput) (*dynamodb.BatchGetItemOutput, error) {
		// Never make progress.
		return &dynamodb.BatchGetItemOutput{
			UnprocessedKeys: map[string]types.KeysAndAttributes{
				"t": {Keys: in.RequestItems["t"].Keys},
			},
		}, nil
	}}
	_, err := BatchGet(context.Background(), fake, "t", []map[string]types.AttributeValue{sItem("P", "a")}, false)
	if !errors.Is(err, ErrUnprocessedRemain) {
		t.Fatalf("want ErrUnprocessedRemain, got %v", err)
	}
	var ue *UnprocessedError
	if !errors.As(err, &ue) || len(ue.Keys) != 1 {
		t.Fatalf("leftover keys not carried: %v", err)
	}
}

func TestBatchWriteRejectsDuplicatesAndChunks(t *testing.T) {
	ctx := context.Background()
	dup := []map[string]types.AttributeValue{sItem("P", "a"), sItem("P", "a")}
	if err := BatchWrite(ctx, &fakeDB{}, "t", dup); !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("want ErrDuplicateKey, got %v", err)
	}

	items := make([]map[string]types.AttributeValue, 0, 30)
	for i := 0; i < 30; i++ {
		items = append(items, sItem("P", string(rune('a'+i))))
	}
	fake := &fakeDB{batchWriteFn: func(in *dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error) {
		return &dynamodb.BatchWriteItemOutput{}, nil
	}}
	if err := BatchWrite(ctx, fake, "t", items); err != nil {
		t.Fatal(err)
	}
	if len(fake.batchWriteIn) != 2 {
		t.Fatalf("expected 2 chunks for 30 items, got %d", len(fake.batchWriteIn))
	}
	if n := len(fake.batchWriteIn[0].RequestItems["t"]); n != 25 {
		t.Fatalf("first chunk should be 25, got %d", n)
	}
}

func TestBatchWriteExhaustedRetries(t *testing.T) {
	fake := &fakeDB{batchWriteFn: func(in *dynamodb.BatchWriteItemInput) (*dynamodb.BatchWriteItemOutput, error) {
		return &dynamodb.BatchWriteItemOutput{
			UnprocessedItems: map[string][]types.WriteRequest{"t": in.RequestItems["t"]},
		}, nil
	}}
	err := BatchWrite(context.Background(), fake, "t", []map[string]types.AttributeValue{sItem("P", "a")})
	var ue *UnprocessedError
	if !errors.As(err, &ue) || len(ue.Writes) != 1 {
		t.Fatalf("leftover writes not carried: %v", err)
	}
}

func TestUpdateExprRemoveOnly(t *testing.T) {
	var e UpdateExpr
	e.Remove("note")
	if e.Empty() {
		t.Fatal("remove-only expr must not be empty")
	}
	if got := e.Expression(); got != "REMOVE #n0" {
		t.Fatalf("expression: %q", got)
	}
	if v := e.Values(nil); v != nil {
		t.Fatalf("remove-only values must be nil (DynamoDB rejects empty maps), got %v", v)
	}
}
