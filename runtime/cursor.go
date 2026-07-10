package runtime

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Cursor is an opaque, URL-safe pagination token wrapping DynamoDB's
// LastEvaluatedKey. The zero value means "from the start"; an empty cursor
// returned from a page means "no more pages".
//
// Cursors are unauthenticated base64 JSON and contain the raw key values of
// the page boundary. Treat them like the keys themselves: fine to hand back
// to the same caller, but do not embed secrets in keys, and expect a
// tampered cursor to surface as a query error rather than a clean rejection.
type Cursor string

// EncodeCursor packs a LastEvaluatedKey into a Cursor. ddbgen key
// attributes are always strings, so only S members are supported.
func EncodeCursor(lek map[string]types.AttributeValue) (Cursor, error) {
	if len(lek) == 0 {
		return "", nil
	}
	m := make(map[string]string, len(lek))
	for k, v := range lek {
		s, ok := v.(*types.AttributeValueMemberS)
		if !ok {
			return "", fmt.Errorf("ddbgen: cursor: key attribute %q is not a string", k)
		}
		m[k] = s.Value
	}
	j, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("ddbgen: encoding cursor: %w", err)
	}
	return Cursor(base64.RawURLEncoding.EncodeToString(j)), nil
}

// Decode unpacks the cursor into an ExclusiveStartKey. An empty cursor
// decodes to nil.
func (c Cursor) Decode() (map[string]types.AttributeValue, error) {
	if c == "" {
		return nil, nil
	}
	j, err := base64.RawURLEncoding.DecodeString(string(c))
	if err != nil {
		return nil, fmt.Errorf("ddbgen: decoding cursor: %w", err)
	}
	var m map[string]string
	if err := json.Unmarshal(j, &m); err != nil {
		return nil, fmt.Errorf("ddbgen: decoding cursor: %w", err)
	}
	out := make(map[string]types.AttributeValue, len(m))
	for k, v := range m {
		out[k] = &types.AttributeValueMemberS{Value: v}
	}
	return out, nil
}
