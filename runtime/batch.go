package runtime

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	batchGetChunk    = 100
	batchWriteChunk  = 25
	maxBatchAttempts = 5
	backoffBase      = 50 * time.Millisecond
)

// backoff sleeps for an exponentially growing, jittered interval, or
// returns the context error when canceled first.
func backoff(ctx context.Context, attempt int) error {
	d := backoffBase << attempt
	d += time.Duration(rand.Int63n(int64(d))) // full jitter on top
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// BatchGet loads items by key, chunking at 100 per request and retrying
// unprocessed keys with jittered exponential backoff. When retries are
// exhausted it returns the items fetched so far along with
// ErrUnprocessedRemain. Missing items are simply absent from the result.
func BatchGet(ctx context.Context, ddb *dynamodb.Client, table string, keys []map[string]types.AttributeValue) ([]map[string]types.AttributeValue, error) {
	var out []map[string]types.AttributeValue
	for start := 0; start < len(keys); start += batchGetChunk {
		chunk := keys[start:min(start+batchGetChunk, len(keys))]
		remaining := chunk
		for attempt := 0; ; attempt++ {
			resp, err := ddb.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
				RequestItems: map[string]types.KeysAndAttributes{table: {Keys: remaining}},
			})
			if err != nil {
				return out, fmt.Errorf("ddbgen: batch get: %w", err)
			}
			out = append(out, resp.Responses[table]...)
			unp := resp.UnprocessedKeys[table].Keys
			if len(unp) == 0 {
				break
			}
			if attempt+1 >= maxBatchAttempts {
				return out, fmt.Errorf("%w: %d keys unprocessed", ErrUnprocessedRemain, len(unp))
			}
			if err := backoff(ctx, attempt); err != nil {
				return out, err
			}
			remaining = unp
		}
	}
	return out, nil
}

// BatchWrite puts items, chunking at 25 per request and retrying
// unprocessed writes with jittered exponential backoff. When retries are
// exhausted it returns ErrUnprocessedRemain.
func BatchWrite(ctx context.Context, ddb *dynamodb.Client, table string, items []map[string]types.AttributeValue) error {
	for start := 0; start < len(items); start += batchWriteChunk {
		chunk := items[start:min(start+batchWriteChunk, len(items))]
		remaining := make([]types.WriteRequest, 0, len(chunk))
		for _, av := range chunk {
			remaining = append(remaining, types.WriteRequest{PutRequest: &types.PutRequest{Item: av}})
		}
		for attempt := 0; ; attempt++ {
			resp, err := ddb.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{table: remaining},
			})
			if err != nil {
				return fmt.Errorf("ddbgen: batch write: %w", err)
			}
			unp := resp.UnprocessedItems[table]
			if len(unp) == 0 {
				break
			}
			if attempt+1 >= maxBatchAttempts {
				return fmt.Errorf("%w: %d writes unprocessed", ErrUnprocessedRemain, len(unp))
			}
			if err := backoff(ctx, attempt); err != nil {
				return err
			}
			remaining = unp
		}
	}
	return nil
}
