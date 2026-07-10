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

// UnprocessedError reports batch items that DynamoDB left unprocessed after
// retries were exhausted. It wraps ErrUnprocessedRemain (match with
// errors.Is) and carries the leftovers so callers can retry them.
type UnprocessedError struct {
	// Keys holds unprocessed BatchGet keys; Writes holds unprocessed
	// BatchWrite requests. Only one is populated, matching the operation.
	Keys   []map[string]types.AttributeValue
	Writes []types.WriteRequest
}

func (e *UnprocessedError) Error() string {
	n := len(e.Keys) + len(e.Writes)
	return fmt.Sprintf("%v: %d left", ErrUnprocessedRemain, n)
}

// Unwrap makes errors.Is(err, ErrUnprocessedRemain) hold.
func (e *UnprocessedError) Unwrap() error { return ErrUnprocessedRemain }

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

// keyFingerprint canonically renders a key map (ddbgen keys are always S
// attributes) for duplicate detection.
func keyFingerprint(key map[string]types.AttributeValue) string {
	pk, sk := "", ""
	for attr, v := range key {
		s, ok := v.(*types.AttributeValueMemberS)
		if !ok {
			continue
		}
		// ddbgen main keys are exactly pk (+ sk); other attrs never occur.
		if attr == "pk" {
			pk = s.Value
		} else {
			sk = s.Value
		}
	}
	return pk + "\x00" + sk
}

// BatchGet loads items by key, chunking at 100 per request and retrying
// unprocessed keys with jittered exponential backoff. Duplicate keys are
// requested once (DynamoDB rejects batches containing duplicates). When
// retries are exhausted it returns the items fetched so far along with an
// *UnprocessedError carrying the leftover keys. Missing items are simply
// absent from the result.
func BatchGet(ctx context.Context, ddb DynamoDB, table string, keys []map[string]types.AttributeValue, consistent bool) ([]map[string]types.AttributeValue, error) {
	seen := make(map[string]bool, len(keys))
	deduped := make([]map[string]types.AttributeValue, 0, len(keys))
	for _, k := range keys {
		fp := keyFingerprint(k)
		if seen[fp] {
			continue
		}
		seen[fp] = true
		deduped = append(deduped, k)
	}
	var out []map[string]types.AttributeValue
	for start := 0; start < len(deduped); start += batchGetChunk {
		chunk := deduped[start:min(start+batchGetChunk, len(deduped))]
		remaining := chunk
		for attempt := 0; ; attempt++ {
			req := types.KeysAndAttributes{Keys: remaining}
			if consistent {
				req.ConsistentRead = &consistent
			}
			resp, err := ddb.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
				RequestItems: map[string]types.KeysAndAttributes{table: req},
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
				return out, &UnprocessedError{Keys: unp}
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
// unprocessed writes with jittered exponential backoff. Two items with the
// same key in one call return ErrDuplicateKey up front — DynamoDB rejects
// such batches, and silently dropping one write would be worse. When
// retries are exhausted it returns an *UnprocessedError carrying the
// leftover write requests.
func BatchWrite(ctx context.Context, ddb DynamoDB, table string, items []map[string]types.AttributeValue) error {
	seen := make(map[string]int, len(items))
	for i, av := range items {
		fp := keyFingerprint(map[string]types.AttributeValue{"pk": av["pk"], "sk": av["sk"]})
		if prev, dup := seen[fp]; dup {
			return fmt.Errorf("%w: items %d and %d write the same key", ErrDuplicateKey, prev, i)
		}
		seen[fp] = i
	}
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
				return &UnprocessedError{Writes: unp}
			}
			if err := backoff(ctx, attempt); err != nil {
				return err
			}
			remaining = unp
		}
	}
	return nil
}
