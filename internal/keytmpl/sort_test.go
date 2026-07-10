package keytmpl

import (
	"bytes"
	"math/rand"
	"testing"
	"time"
)

// TestSortability proves the fixed-width claim: for 1k random pairs per
// encoder, lexicographic order of encodings matches semantic order of the
// source values.
func TestSortability(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	const pairs = 1000

	t.Run("rfc3339", func(t *testing.T) {
		seg := Segment{Kind: SegPlaceholder, Field: "F", Encoder: "rfc3339"}
		for i := 0; i < pairs; i++ {
			a := time.Unix(rng.Int63n(4_000_000_000), rng.Int63n(1_000_000_000)).UTC()
			b := time.Unix(rng.Int63n(4_000_000_000), rng.Int63n(1_000_000_000)).UTC()
			checkOrder(t, seg, a, b, a.Before(b), a.Equal(b))
		}
	})
	t.Run("epoch", func(t *testing.T) {
		seg := Segment{Kind: SegPlaceholder, Field: "F", Encoder: "epoch"}
		for i := 0; i < pairs; i++ {
			a, b := rng.Int63n(999_999_999_999), rng.Int63n(999_999_999_999)
			checkOrder(t, seg, a, b, a < b, a == b)
		}
	})
	t.Run("epochms", func(t *testing.T) {
		seg := Segment{Kind: SegPlaceholder, Field: "F", Encoder: "epochms"}
		for i := 0; i < pairs; i++ {
			a, b := rng.Int63n(999_999_999_999_999), rng.Int63n(999_999_999_999_999)
			checkOrder(t, seg, a, b, a < b, a == b)
		}
	})
	t.Run("pad12", func(t *testing.T) {
		seg := Segment{Kind: SegPlaceholder, Field: "F", Encoder: "pad12"}
		for i := 0; i < pairs; i++ {
			a, b := rng.Int63n(999_999_999_999), rng.Int63n(999_999_999_999)
			checkOrder(t, seg, a, b, a < b, a == b)
		}
	})
	t.Run("hex fixed width", func(t *testing.T) {
		seg := Segment{Kind: SegPlaceholder, Field: "F", Encoder: "hex"}
		for i := 0; i < pairs; i++ {
			a, b := make([]byte, 8), make([]byte, 8)
			rng.Read(a)
			rng.Read(b)
			cmp := bytes.Compare(a, b)
			checkOrder(t, seg, a, b, cmp < 0, cmp == 0)
		}
	})
	t.Run("ulid", func(t *testing.T) {
		const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
		seg := Segment{Kind: SegPlaceholder, Field: "F", Encoder: "ulid"}
		gen := func() string {
			b := make([]byte, 26)
			b[0] = crockford[rng.Intn(8)]
			for i := 1; i < 26; i++ {
				b[i] = crockford[rng.Intn(len(crockford))]
			}
			return string(b)
		}
		for i := 0; i < pairs; i++ {
			a, b := gen(), gen()
			checkOrder(t, seg, a, b, a < b, a == b)
		}
	})

	// Full-template ordering across the delimiter: two-segment keys where
	// the first placeholder is fixed-width must sort by (first, second).
	t.Run("template with delimiter", func(t *testing.T) {
		tmpl := mustParse(t, "ORDER#{At:epoch}#{ID}")
		for i := 0; i < pairs; i++ {
			atA, atB := rng.Int63n(1_000_000), rng.Int63n(1_000_000)
			idA, idB := string(rune('a'+rng.Intn(26))), string(rune('a'+rng.Intn(26)))
			keyA, err := tmpl.Encode(map[string]any{"At": atA, "ID": idA})
			if err != nil {
				t.Fatal(err)
			}
			keyB, err := tmpl.Encode(map[string]any{"At": atB, "ID": idB})
			if err != nil {
				t.Fatal(err)
			}
			semLess := atA < atB || (atA == atB && idA < idB)
			if (keyA < keyB) != semLess && keyA != keyB {
				t.Fatalf("template order broken: (%d,%s)=%q vs (%d,%s)=%q", atA, idA, keyA, atB, idB, keyB)
			}
		}
	})
}

func checkOrder(t *testing.T, seg Segment, a, b any, semLess, semEqual bool) {
	t.Helper()
	ea, err := EncodeSegment(seg, a)
	if err != nil {
		t.Fatalf("%s: encode %v: %v", seg.Encoder, a, err)
	}
	eb, err := EncodeSegment(seg, b)
	if err != nil {
		t.Fatalf("%s: encode %v: %v", seg.Encoder, b, err)
	}
	switch {
	case semEqual:
		if ea != eb {
			t.Fatalf("%s: equal values encode differently: %q vs %q", seg.Encoder, ea, eb)
		}
	case semLess:
		if ea >= eb {
			t.Fatalf("%s: order broken: %v < %v but %q >= %q", seg.Encoder, a, b, ea, eb)
		}
	default:
		if ea <= eb {
			t.Fatalf("%s: order broken: %v > %v but %q <= %q", seg.Encoder, a, b, ea, eb)
		}
	}
}
