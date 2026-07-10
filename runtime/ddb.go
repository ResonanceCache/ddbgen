package runtime

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// IsConditionalCheckFailed reports whether err is DynamoDB's
// ConditionalCheckFailedException. Generated code maps it onto
// ErrConditionFailed or ErrVersionConflict depending on the condition.
func IsConditionalCheckFailed(err error) bool {
	var ccf *types.ConditionalCheckFailedException
	return errors.As(err, &ccf)
}

// IsTransactionConditionFailed reports whether err is a transaction
// cancellation caused by a failed condition check.
func IsTransactionConditionFailed(err error) bool {
	var tc *types.TransactionCanceledException
	if !errors.As(err, &tc) {
		return false
	}
	for _, r := range tc.CancellationReasons {
		if r.Code != nil && *r.Code == "ConditionalCheckFailed" {
			return true
		}
	}
	return false
}
