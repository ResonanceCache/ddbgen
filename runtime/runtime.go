// Package runtime is the small support library imported by ddbgen-generated
// code: key-segment encoders, opaque pagination cursors, query iteration,
// batch chunking with retry, and sentinel errors. It contains no reflection
// and no code paths outside what generated clients call.
package runtime

import "errors"

// Delimiter joins key template segments in physical key strings.
const Delimiter = "#"

// MaxKeySuffix sorts after every encoded key segment byte sequence. Range
// bounds append it to make "after" and inclusive "between" cuts precise at
// placeholder boundaries.
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
	// ErrUnprocessedRemain is returned when batch retries are exhausted and
	// unprocessed items remain.
	ErrUnprocessedRemain = errors.New("ddbgen: unprocessed items remain after retries")
)
