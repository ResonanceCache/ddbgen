package runtime

import (
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// UpdateExpr accumulates a DynamoDB update expression with hygienic
// aliases: every attribute name becomes #n<i> and every value :u<i>, so
// reserved words can never collide.
type UpdateExpr struct {
	sets    []string
	removes []string
	adds    []string
	names   map[string]string
	values  map[string]types.AttributeValue
	n       int
}

func (e *UpdateExpr) init() {
	if e.names == nil {
		e.names = map[string]string{}
		e.values = map[string]types.AttributeValue{}
	}
}

func (e *UpdateExpr) next() int {
	e.init()
	n := e.n
	e.n++
	return n
}

func (e *UpdateExpr) alias(attr string) string {
	name := "#n" + strconv.Itoa(e.next())
	e.names[name] = attr
	return name
}

func (e *UpdateExpr) bind(v types.AttributeValue) string {
	val := ":u" + strconv.Itoa(e.next())
	e.values[val] = v
	return val
}

// Set appends "attr = v".
func (e *UpdateExpr) Set(attr string, v types.AttributeValue) {
	name := e.alias(attr)
	e.sets = append(e.sets, name+" = "+e.bind(v))
}

// Remove appends a REMOVE of attr.
func (e *UpdateExpr) Remove(attr string) {
	e.removes = append(e.removes, e.alias(attr))
}

// Add appends an ADD of a numeric delta to attr.
func (e *UpdateExpr) Add(attr string, v types.AttributeValue) {
	name := e.alias(attr)
	e.adds = append(e.adds, name+" "+e.bind(v))
}

// IncrementVersion appends "attr = if_not_exists(attr, 0) + 1",
// initializing items that predate versioning.
func (e *UpdateExpr) IncrementVersion(attr string) {
	name := e.alias(attr)
	zero := e.bind(&types.AttributeValueMemberN{Value: "0"})
	one := e.bind(&types.AttributeValueMemberN{Value: "1"})
	e.sets = append(e.sets, name+" = if_not_exists("+name+", "+zero+") + "+one)
}

// Empty reports whether no clauses were added.
func (e *UpdateExpr) Empty() bool {
	return len(e.sets) == 0 && len(e.removes) == 0 && len(e.adds) == 0
}

// Expression renders the full update expression.
func (e *UpdateExpr) Expression() string {
	var parts []string
	if len(e.sets) > 0 {
		parts = append(parts, "SET "+strings.Join(e.sets, ", "))
	}
	if len(e.removes) > 0 {
		parts = append(parts, "REMOVE "+strings.Join(e.removes, ", "))
	}
	if len(e.adds) > 0 {
		parts = append(parts, "ADD "+strings.Join(e.adds, ", "))
	}
	return strings.Join(parts, " ")
}

// Names returns the accumulated attribute-name aliases, merging in any
// extra aliases the caller needs for condition expressions.
func (e *UpdateExpr) Names(extra map[string]string) map[string]string {
	e.init()
	for k, v := range extra {
		e.names[k] = v
	}
	return e.names
}

// Values returns the accumulated value bindings, merging in any extras.
// Returns nil when empty, as the DynamoDB API requires.
func (e *UpdateExpr) Values(extra map[string]types.AttributeValue) map[string]types.AttributeValue {
	e.init()
	for k, v := range extra {
		e.values[k] = v
	}
	if len(e.values) == 0 {
		return nil
	}
	return e.values
}
