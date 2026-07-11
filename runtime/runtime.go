// Package runtime is the small support library imported by ddbgen-generated
// code: key-segment encoders, opaque pagination cursors, query iteration,
// batch chunking with retry, and sentinel errors. It contains no reflection
// and no code paths outside what generated clients call.
package runtime

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

// DynamoDB is the subset of the DynamoDB client used by generated code, the
// runtime helpers, and the generated client's DynamoDB() escape hatch.
// *dynamodb.Client implements it; substitute a mock or a wrapping
// implementation to test code that uses a generated client, per the AWS SDK
// for Go v2 testing guidance.
//
// Query through TransactWriteItems are what generated code calls. Scan is
// not — no generated method scans — but it is included so the DynamoDB()
// escape hatch can reach the one table-wide read that key-based access
// cannot express (admin counts, migrations, full-table audits). The
// interface stays small and mockable; it is deliberately not a kitchen-sink
// DynamoDBAPI.
type DynamoDB interface {
	GetItem(ctx context.Context, in *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, in *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	DeleteItem(ctx context.Context, in *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	UpdateItem(ctx context.Context, in *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	Query(ctx context.Context, in *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, in *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	BatchGetItem(ctx context.Context, in *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
	BatchWriteItem(ctx context.Context, in *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	TransactWriteItems(ctx context.Context, in *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
}

// Delimiter joins key template segments in physical key strings.
const Delimiter = "#"

// MaxKeySuffix sorts after every byte sequence the fixed-width encoders and
// the delimiter can produce (their alphabets stay below U+FFFF's UTF-8
// encoding, 0xEF 0xBF 0xBF). Range bounds append it to make "after" and
// inclusive "between" cuts precise at placeholder boundaries. Raw string
// segments containing supplementary-plane runes (four-byte UTF-8, e.g.
// emoji) sort above it, but range bounds only ever abut fixed-width
// encodings, never raw segments.
const MaxKeySuffix = "\uffff"

var (
	// ErrNotFound is returned when a Get finds no item.
	ErrNotFound = errors.New("ddbgen: item not found")
	// ErrConditionFailed wraps DynamoDB's ConditionalCheckFailedException
	// for non-version conditions (e.g. PutIfNotExists on an existing item).
	ErrConditionFailed = errors.New("ddbgen: condition failed")
	// ErrVersionConflict is returned when an optimistic-locking version
	// condition fails: the item changed since it was read.
	ErrVersionConflict = errors.New("ddbgen: version conflict")
	// ErrDelimiterInValue is returned when an encoded key segment contains
	// the key delimiter. Use the urlenc encoder for delimiter-bearing values.
	ErrDelimiterInValue = errors.New("ddbgen: value contains key delimiter")
	// ErrEmptySegment is returned when a key segment encodes to the empty
	// string, which would produce a malformed physical key.
	ErrEmptySegment = errors.New("ddbgen: key segment encodes to empty string")
	// ErrUnprocessedRemain is returned (inside an *UnprocessedError) when
	// batch retries are exhausted and unprocessed items remain.
	ErrUnprocessedRemain = errors.New("ddbgen: unprocessed items remain after retries")
	// ErrDuplicateKey is returned when a batch contains two operations on
	// the same key, which DynamoDB rejects wholesale.
	ErrDuplicateKey = errors.New("ddbgen: duplicate key in batch")
)
