package keytmpl

import (
	"strings"
	"testing"
)

func orderTypes(field string) (string, bool) {
	types := map[string]string{
		"CreatedAt": "time.Time",
		"OrderID":   "string",
		"PayID":     "string",
		"Sum":       "[8]byte",
	}
	t, ok := types[field]
	return t, ok
}

func mustPrefix(t *testing.T, raw string) *Prefix {
	t.Helper()
	p, err := ParsePrefix(raw)
	if err != nil {
		t.Fatalf("ParsePrefix(%q): %v", raw, err)
	}
	return p
}

func TestAlignPrefix(t *testing.T) {
	sk := mustParse(t, "ORDER#{CreatedAt:rfc3339}#{OrderID}")

	tests := []struct {
		prefix   string
		consumed int
		next     string // field name of Next, "" for nil
		errSub   string
	}{
		{"ORDER#", 1, "CreatedAt", ""},
		{"ORDER", 1, "CreatedAt", ""},
		{"ORDER#{CreatedAt:rfc3339}#", 2, "OrderID", ""},
		{"ORDER#{CreatedAt:rfc3339}", 2, "OrderID", ""}, // fixed-width may end without delimiter
		{"ORD", 0, "", "mid-literal"},
		{"PAY#", 0, "", "does not match"},
		{"ORDER#{UpdatedAt:rfc3339}#", 0, "", "does not match"},
		{"ORDER#{CreatedAt:epoch}#", 0, "", "does not match"},
		{"ORDER#{CreatedAt:rfc3339}#{OrderID}#extra", 0, "", "more segments"},
	}
	for _, tt := range tests {
		cut, err := AlignPrefix(sk, mustPrefix(t, tt.prefix), orderTypes)
		if tt.errSub != "" {
			if err == nil || !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("AlignPrefix(%q): want error containing %q, got %v", tt.prefix, tt.errSub, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("AlignPrefix(%q): %v", tt.prefix, err)
			continue
		}
		if cut.Consumed != tt.consumed {
			t.Errorf("AlignPrefix(%q): consumed %d, want %d", tt.prefix, cut.Consumed, tt.consumed)
		}
		next := ""
		if cut.Next != nil {
			next = cut.Next.Field
		}
		if next != tt.next {
			t.Errorf("AlignPrefix(%q): next %q, want %q", tt.prefix, next, tt.next)
		}
	}
}

func TestAlignPrefixVariableWidthEnd(t *testing.T) {
	sk := mustParse(t, "PAY#{OrderID}#{PayID}")
	// Ending inside a variable-width placeholder without a delimiter anchor
	// must be rejected; with the trailing delimiter it is precise.
	if _, err := AlignPrefix(sk, mustPrefix(t, "PAY#{OrderID}"), orderTypes); err == nil {
		t.Fatal("expected variable-width mid-value rejection")
	}
	cut, err := AlignPrefix(sk, mustPrefix(t, "PAY#{OrderID}#"), orderTypes)
	if err != nil {
		t.Fatal(err)
	}
	if cut.Consumed != 2 || cut.Next == nil || cut.Next.Field != "PayID" {
		t.Fatalf("unexpected cut: %+v", cut)
	}
	if cut.RangeEligible(orderTypes) {
		t.Fatal("PayID is a raw string; range methods must not be eligible")
	}
}

func TestLeadingLiteralCut(t *testing.T) {
	sk := mustParse(t, "ORDER#{CreatedAt:rfc3339}#{OrderID}")
	cut := LeadingLiteralCut(sk)
	if cut.Consumed != 1 || cut.Next == nil || cut.Next.Field != "CreatedAt" || cut.NextIsFinal {
		t.Fatalf("unexpected cut: %+v", cut)
	}
	if !cut.RangeEligible(orderTypes) {
		t.Fatal("rfc3339 must be range-eligible")
	}
	if got := PrefixString(sk, cut.Consumed); got != "ORDER#" {
		t.Fatalf("PrefixString = %q, want ORDER#", got)
	}

	bare := mustParse(t, "{CreatedAt:rfc3339}")
	cut = LeadingLiteralCut(bare)
	if cut.Consumed != 0 || cut.Next == nil || !cut.NextIsFinal {
		t.Fatalf("unexpected cut for bare placeholder: %+v", cut)
	}
	if got := PrefixString(bare, 0); got != "" {
		t.Fatalf("PrefixString = %q, want empty", got)
	}

	allLit := mustParse(t, "META#CONFIG")
	cut = LeadingLiteralCut(allLit)
	if cut.Consumed != 2 || cut.Next != nil {
		t.Fatalf("unexpected cut for all-literal template: %+v", cut)
	}
	if got := PrefixString(allLit, 2); got != "META#CONFIG" {
		t.Fatalf("PrefixString = %q", got)
	}
}
