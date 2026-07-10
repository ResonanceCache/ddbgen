package keytmpl

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func mustParse(t *testing.T, raw string) *Template {
	t.Helper()
	tmpl, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%q): %v", raw, err)
	}
	return tmpl
}

func TestParseRejects(t *testing.T) {
	for _, raw := range []string{
		"",
		"ORDER##{ID}",
		"#ORDER",
		"ORDER#",
		"ORD{ID}",
		"{id lower}",
		"{ID:zip}",
		"{:upper}",
	} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q): expected error", raw)
		}
	}
}

func TestEncodeDecodeTable(t *testing.T) {
	ts := time.Date(2026, 7, 9, 12, 30, 45, 123456789, time.UTC)
	tests := []struct {
		tmpl string
		vals map[string]any
		want string
	}{
		{"TENANT#{TenantID}", map[string]any{"TenantID": "t1"}, "TENANT#t1"},
		{
			"ORDER#{CreatedAt:rfc3339}#{OrderID}",
			map[string]any{"CreatedAt": ts, "OrderID": "o1"},
			"ORDER#2026-07-09T12:30:45.123456789Z#o1",
		},
		{"{UpdatedAt:epoch}", map[string]any{"UpdatedAt": ts}, "001783600245"},
		{"{MS:epochms}", map[string]any{"MS": int64(1_783_686_645_123)}, "001783686645123"},
		{"N#{Num:pad6}", map[string]any{"Num": int64(42)}, "N#000042"},
		{"S#{Status:upper}", map[string]any{"Status": "shipped"}, "S#SHIPPED"},
		{"S#{Status:lower}", map[string]any{"Status": "SHIPPED"}, "S#shipped"},
		{"H#{Sum:hex}", map[string]any{"Sum": []byte{0xde, 0xad}}, "H#dead"},
		{"U#{ID:ulid}", map[string]any{"ID": "01arz3ndektsv4rrffq69g5fav"}, "U#01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{"E#{Name:urlenc}", map[string]any{"Name": "a#b c"}, "E#a%23b+c"},
	}
	for _, tt := range tests {
		tmpl := mustParse(t, tt.tmpl)
		got, err := tmpl.Encode(tt.vals)
		if err != nil {
			t.Errorf("Encode(%q): %v", tt.tmpl, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Encode(%q) = %q, want %q", tt.tmpl, got, tt.want)
		}
		back, err := tmpl.Decode(got)
		if err != nil {
			t.Errorf("Decode(%q, %q): %v", tt.tmpl, got, err)
		}
		re, err := tmpl.Encode(mergeCanonical(tt.vals, back))
		if err != nil || re != got {
			t.Errorf("re-Encode after Decode(%q) = %q, %v; want %q", tt.tmpl, re, err, got)
		}
	}
}

// mergeCanonical overlays decoded canonical values on the original input so
// re-encoding proves the decode inverse (upper/lower normalize, epoch
// collapses time.Time to seconds).
func mergeCanonical(orig, decoded map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range orig {
		out[k] = v
	}
	for k, v := range decoded {
		out[k] = v
	}
	return out
}

func TestEncodeRejectsDelimiter(t *testing.T) {
	tmpl := mustParse(t, "TENANT#{TenantID}")
	if _, err := tmpl.Encode(map[string]any{"TenantID": "a#b"}); err == nil {
		t.Fatal("expected delimiter error for raw string containing '#'")
	}
	if _, err := tmpl.Encode(map[string]any{}); err == nil {
		t.Fatal("expected missing-value error")
	}
}

// TestRoundTripProperty is a hand-rolled property test: random values per
// encoder must survive encode -> decode -> encode identically.
func TestRoundTripProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	const rounds = 1000

	randTime := func() time.Time {
		return time.Unix(rng.Int63n(4_000_000_000), rng.Int63n(1_000_000_000)).UTC()
	}
	randString := func() string {
		const alpha = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.:/ %+"
		n := 1 + rng.Intn(20)
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteByte(alpha[rng.Intn(len(alpha))])
		}
		return b.String()
	}
	randULID := func() string {
		const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
		b := make([]byte, 26)
		b[0] = crockford[rng.Intn(8)]
		for i := 1; i < 26; i++ {
			b[i] = crockford[rng.Intn(len(crockford))]
		}
		return string(b)
	}

	for i := 0; i < rounds; i++ {
		checkRoundTrip(t, Segment{Kind: SegPlaceholder, Field: "F", Encoder: "rfc3339"}, randTime())
		checkRoundTrip(t, Segment{Kind: SegPlaceholder, Field: "F", Encoder: "epoch"}, rng.Int63n(999_999_999_999))
		checkRoundTrip(t, Segment{Kind: SegPlaceholder, Field: "F", Encoder: "epochms"}, rng.Int63n(999_999_999_999_999))
		checkRoundTrip(t, Segment{Kind: SegPlaceholder, Field: "F", Encoder: "pad9"}, rng.Int63n(1_000_000_000))
		checkRoundTrip(t, Segment{Kind: SegPlaceholder, Field: "F", Encoder: "ulid"}, randULID())
		checkRoundTrip(t, Segment{Kind: SegPlaceholder, Field: "F", Encoder: "urlenc"}, randString())
		sum := make([]byte, 8)
		rng.Read(sum)
		checkRoundTrip(t, Segment{Kind: SegPlaceholder, Field: "F", Encoder: "hex"}, sum)
		checkRoundTrip(t, Segment{Kind: SegPlaceholder, Field: "F", Encoder: "upper"}, randString())
		checkRoundTrip(t, Segment{Kind: SegPlaceholder, Field: "F", Encoder: "lower"}, randString())
	}
}

func checkRoundTrip(t *testing.T, seg Segment, v any) {
	t.Helper()
	enc, err := EncodeSegment(seg, v)
	if err != nil {
		t.Fatalf("%s: EncodeSegment(%v): %v", seg.Encoder, v, err)
	}
	dec, err := DecodeSegment(seg, enc)
	if err != nil {
		t.Fatalf("%s: DecodeSegment(%q): %v", seg.Encoder, enc, err)
	}
	re, err := EncodeSegment(seg, dec)
	if err != nil {
		t.Fatalf("%s: re-EncodeSegment(%v): %v", seg.Encoder, dec, err)
	}
	if re != enc {
		t.Fatalf("%s: round trip drift: %q -> %v -> %q", seg.Encoder, enc, dec, re)
	}
	// Strong inverse for lossless encoders.
	switch seg.Encoder {
	case "rfc3339":
		if !dec.(time.Time).Equal(v.(time.Time)) {
			t.Fatalf("rfc3339: decode(%q) = %v, want %v", enc, dec, v)
		}
	case "epoch", "epochms", "pad9":
		if dec.(int64) != v.(int64) {
			t.Fatalf("%s: decode(%q) = %v, want %v", seg.Encoder, enc, dec, v)
		}
	case "urlenc":
		if dec.(string) != v.(string) {
			t.Fatalf("urlenc: decode(%q) = %q, want %q", enc, dec, v)
		}
	case "hex":
		if !bytes.Equal(dec.([]byte), v.([]byte)) {
			t.Fatalf("hex: decode(%q) = %v, want %v", enc, dec, v)
		}
	}
}
