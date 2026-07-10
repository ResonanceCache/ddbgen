package runtime

import (
	"strings"
	"testing"
	"time"
)

func TestCheckSegment(t *testing.T) {
	if _, err := CheckSegment("a#b"); err == nil {
		t.Fatal("expected delimiter rejection")
	}
	if s, err := CheckSegment("ab"); err != nil || s != "ab" {
		t.Fatalf("got %q, %v", s, err)
	}
}

func TestEncodeRFC3339Bounds(t *testing.T) {
	if _, err := EncodeRFC3339(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected year 10000 rejection")
	}
	if _, err := EncodeRFC3339(time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("expected year 0 rejection")
	}
	s, err := EncodeRFC3339(time.Date(2026, 7, 9, 1, 2, 3, 4, time.FixedZone("X", 3600)))
	if err != nil {
		t.Fatal(err)
	}
	if len(s) != 30 || !strings.HasSuffix(s, "Z") {
		t.Fatalf("not fixed-width UTC: %q", s)
	}
	if s != "2026-07-09T00:02:03.000000004Z" {
		t.Fatalf("timezone not normalized: %q", s)
	}
}

func TestEpochRejects(t *testing.T) {
	if _, err := EncodeEpoch(-1); err == nil {
		t.Fatal("expected negative rejection")
	}
	if _, err := EncodeEpoch(1_000_000_000_000); err == nil {
		t.Fatal("expected width-12 overflow rejection")
	}
	if _, err := EncodeEpochMS(-5); err == nil {
		t.Fatal("expected negative rejection")
	}
	s, err := EncodeEpoch(0)
	if err != nil || s != "000000000000" {
		t.Fatalf("EncodeEpoch(0) = %q, %v", s, err)
	}
}

func TestPadRejects(t *testing.T) {
	if _, err := EncodePad(-1, 4); err == nil {
		t.Fatal("expected negative rejection")
	}
	if _, err := EncodePad(10000, 4); err == nil {
		t.Fatal("expected overflow rejection")
	}
	if s, _ := EncodePad(7, 4); s != "0007" {
		t.Fatalf("EncodePad(7,4) = %q", s)
	}
	if s, _ := EncodePadUint(7, 4); s != "0007" {
		t.Fatalf("EncodePadUint(7,4) = %q", s)
	}
}

func TestULIDValidation(t *testing.T) {
	valid := "01arz3ndektsv4rrffq69g5fav"
	u, err := EncodeULID(valid)
	if err != nil {
		t.Fatal(err)
	}
	if u != strings.ToUpper(valid) {
		t.Fatalf("not uppercased: %q", u)
	}
	for _, bad := range []string{
		"short",
		"81ARZ3NDEKTSV4RRFFQ69G5FAV", // first char > 7
		"01ARZ3NDEKTSV4RRFFQ69G5FAL", // L not in alphabet
		"01ARZ3NDEKTSV4RRFFQ69G5FA#",
	} {
		if _, err := EncodeULID(bad); err == nil {
			t.Errorf("EncodeULID(%q): expected error", bad)
		}
	}
}

func TestPredecessors(t *testing.T) {
	ts := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	enc, _ := EncodeRFC3339(ts)
	pred, ok := PredRFC3339(ts)
	if !ok || pred >= enc {
		t.Fatalf("PredRFC3339: %q not below %q", pred, enc)
	}
	if want, _ := EncodeRFC3339(ts.Add(-time.Nanosecond)); pred != want {
		t.Fatalf("PredRFC3339 = %q, want %q", pred, want)
	}

	if p, ok := PredEpoch(1); !ok || p != "000000000000" {
		t.Fatalf("PredEpoch(1) = %q, %v", p, ok)
	}
	if _, ok := PredEpoch(0); ok {
		t.Fatal("PredEpoch(0) must underflow")
	}
	if p, ok := PredPad(100, 6); !ok || p != "000099" {
		t.Fatalf("PredPad(100,6) = %q, %v", p, ok)
	}

	if p, ok := PredULID("01ARZ3NDEKTSV4RRFFQ69G5FA0"); !ok || p != "01ARZ3NDEKTSV4RRFFQ69G5F9Z" {
		t.Fatalf("PredULID rollover = %q, %v", p, ok)
	}
	if _, ok := PredULID("00000000000000000000000000"); ok {
		t.Fatal("all-zero ULID must underflow")
	}

	if p, ok := PredHex("1000"); !ok || p != "0fff" {
		t.Fatalf("PredHex(1000) = %q, %v", p, ok)
	}
	if _, ok := PredHex("0000"); ok {
		t.Fatal("all-zero hex must underflow")
	}
}
