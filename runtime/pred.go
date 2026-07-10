package runtime

import (
	"strings"
	"time"
)

// Predecessor functions return the encoding of the largest encodable value
// strictly below the given one. Generated Before(...) methods use them to
// build exclusive upper bounds out of DynamoDB's inclusive BETWEEN. The
// boolean is false on underflow (nothing sorts below the value), which
// callers turn into a provably empty range.

// PredRFC3339 returns the encoding of t minus one nanosecond.
func PredRFC3339(t time.Time) (string, bool) {
	p := t.UTC().Add(-time.Nanosecond)
	if p.After(t.UTC()) || p.Year() < 1 { // wrapped or left encodable range
		return "", false
	}
	s, err := EncodeRFC3339(p)
	if err != nil {
		return "", false
	}
	return s, true
}

// PredEpoch returns the encoding of sec-1.
func PredEpoch(sec int64) (string, bool) {
	if sec <= 0 {
		return "", false
	}
	s, err := EncodeEpoch(sec - 1)
	if err != nil {
		return "", false
	}
	return s, true
}

// PredEpochMS returns the encoding of ms-1.
func PredEpochMS(ms int64) (string, bool) {
	if ms <= 0 {
		return "", false
	}
	s, err := EncodeEpochMS(ms - 1)
	if err != nil {
		return "", false
	}
	return s, true
}

// PredPad returns the encoding of v-1 at the given width.
func PredPad(v int64, width int) (string, bool) {
	if v <= 0 {
		return "", false
	}
	s, err := EncodePad(v-1, width)
	if err != nil {
		return "", false
	}
	return s, true
}

// PredPadUint returns the encoding of v-1 at the given width.
func PredPadUint(v uint64, width int) (string, bool) {
	if v == 0 {
		return "", false
	}
	s, err := EncodePadUint(v-1, width)
	if err != nil {
		return "", false
	}
	return s, true
}

// PredULID returns the 26-character Crockford string immediately below the
// given ULID.
func PredULID(s string) (string, bool) {
	u, err := EncodeULID(s)
	if err != nil {
		return "", false
	}
	return predAlphabet(u, crockford)
}

// PredHex returns the lowercase hex string immediately below the given one,
// preserving its width.
func PredHex(s string) (string, bool) {
	return predAlphabet(strings.ToLower(s), "0123456789abcdef")
}

// predAlphabet decrements a fixed-width string over a sorted alphabet:
// the rightmost non-minimum character steps down one place and everything
// after it saturates to the maximum character.
func predAlphabet(s, alphabet string) (string, bool) {
	min, max := alphabet[0], alphabet[len(alphabet)-1]
	b := []byte(s)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == min {
			b[i] = max
			continue
		}
		pos := strings.IndexByte(alphabet, b[i])
		if pos < 0 {
			return "", false
		}
		b[i] = alphabet[pos-1]
		return string(b), true
	}
	return "", false // all-minimum: nothing sorts below
}
