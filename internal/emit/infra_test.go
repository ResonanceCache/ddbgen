package emit

import (
	"strings"
	"testing"

	"github.com/ResonanceCache/ddbgen/internal/parser"
)

func TestTTLAttrConflict(t *testing.T) {
	src := `package p

//ddb:entity table=app type=a ttl=ExpA
//ddb:key pk="A#{ID}"
type A struct {
	ID   string ` + "`dynamodbav:\"id\"`" + `
	ExpA int64  ` + "`dynamodbav:\"exp_a\"`" + `
}

//ddb:entity table=app type=b ttl=ExpB
//ddb:key pk="B#{ID}"
type B struct {
	ID   string ` + "`dynamodbav:\"id\"`" + `
	ExpB int64  ` + "`dynamodbav:\"exp_b\"`" + `
}
`
	s, err := parser.ParseSource("input.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CloudFormation(s.Tables[0]); err == nil || !strings.Contains(err.Error(), "one TTL attribute") {
		t.Fatalf("conflicting TTL attributes must fail infra emit, got %v", err)
	}
	if _, err := Terraform(s.Tables[0]); err == nil {
		t.Fatal("Terraform emit must fail on TTL conflict too")
	}
}

func TestResourceNameSanitization(t *testing.T) {
	tests := map[string]string{
		"app":         "AppTable",
		"orders_v2":   "OrdersV2Table",
		"my-table":    "MyTableTable",
		"2fast":       "Ddb2fastTable",
		"snake_case_": "SnakeCaseTable",
	}
	for in, want := range tests {
		if got := resourceName(in); got != want {
			t.Errorf("resourceName(%q) = %q, want %q", in, got, want)
		}
	}
}
