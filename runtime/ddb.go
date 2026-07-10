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
