package runtime

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// rfc3339Layout is fixed-width: forced UTC, forced 9-digit nanoseconds.
// Anything narrower breaks lexicographic ordering of encoded keys.
const rfc3339Layout = "2006-01-02T15:04:05.000000000Z"

// CheckSegment rejects raw string segment values containing the delimiter.
func CheckSegment(s string) (string, error) {
	if strings.Contains(s, Delimiter) {
		return "", fmt.Errorf("%w: %q", ErrDelimiterInValue, s)
	}
	return s, nil
}

// EncodeRFC3339 encodes a timestamp as fixed-width RFC 3339 in UTC with
// 9-digit nanoseconds. Years outside [1, 9999] are rejected: they change
// the encoded width.
func EncodeRFC3339(t time.Time) (string, error) {
	u := t.UTC()
	if y := u.Year(); y < 1 || y > 9999 {
		return "", fmt.Errorf("ddbgen: rfc3339 encoder requires year in [1, 9999], got %d", y)
	}
	return u.Format(rfc3339Layout), nil
}

// DecodeRFC3339 is the inverse of EncodeRFC3339.
func DecodeRFC3339(s string) (time.Time, error) {
	t, err := time.Parse(rfc3339Layout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("ddbgen: decoding rfc3339 key segment: %w", err)
	}
	return t, nil
}

// EncodeEpoch encodes non-negative Unix seconds zero-padded to width 12.
func EncodeEpoch(sec int64) (string, error) {
	return padInt(sec, 12, "epoch")
}

// EncodeEpochTime encodes a timestamp's Unix seconds via EncodeEpoch.
func EncodeEpochTime(t time.Time) (string, error) {
	return EncodeEpoch(t.Unix())
}

// DecodeEpoch is the inverse of EncodeEpoch.
func DecodeEpoch(s string) (int64, error) {
	return unpadInt(s, 12, "epoch")
}

// EncodeEpochMS encodes non-negative Unix milliseconds zero-padded to width 15.
func EncodeEpochMS(ms int64) (string, error) {
	return padInt(ms, 15, "epochms")
}

// EncodeEpochMSTime encodes a timestamp's Unix milliseconds via EncodeEpochMS.
func EncodeEpochMSTime(t time.Time) (string, error) {
	return EncodeEpochMS(t.UnixMilli())
}

// DecodeEpochMS is the inverse of EncodeEpochMS.
func DecodeEpochMS(s string) (int64, error) {
	return unpadInt(s, 15, "epochms")
}

// EncodePad zero-pads a non-negative integer to the given width.
func EncodePad(v int64, width int) (string, error) {
	return padInt(v, width, "pad")
}

// EncodePadUint zero-pads an unsigned integer to the given width.
func EncodePadUint(v uint64, width int) (string, error) {
	s := strconv.FormatUint(v, 10)
	if len(s) > width {
		return "", fmt.Errorf("ddbgen: pad encoder overflow: %d does not fit width %d", v, width)
	}
	return strings.Repeat("0", width-len(s)) + s, nil
}

// DecodePad is the inverse of EncodePad for a known width.
func DecodePad(s string, width int) (int64, error) {
	return unpadInt(s, width, "pad")
}

func padInt(v int64, width int, name string) (string, error) {
	if v < 0 {
		return "", fmt.Errorf("ddbgen: %s encoder rejects negative value %d", name, v)
	}
	s := strconv.FormatInt(v, 10)
	if len(s) > width {
		return "", fmt.Errorf("ddbgen: %s encoder overflow: %d does not fit width %d", name, v, width)
	}
	return strings.Repeat("0", width-len(s)) + s, nil
}

func unpadInt(s string, width int, name string) (int64, error) {
	if len(s) != width {
		return 0, fmt.Errorf("ddbgen: decoding %s key segment: want width %d, got %d", name, width, len(s))
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("ddbgen: decoding %s key segment: %w", name, err)
	}
	return v, nil
}

// EncodeHex encodes bytes as lowercase hex.
func EncodeHex(b []byte) string {
	return hex.EncodeToString(b)
}

// DecodeHex is the inverse of EncodeHex.
func DecodeHex(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("ddbgen: decoding hex key segment: %w", err)
	}
	return b, nil
}

// crockford is the ULID alphabet: 0-9 plus uppercase letters minus I, L, O, U.
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// EncodeULID validates a 26-character Crockford base32 ULID and uppercases
// it. Format check only; no ULID dependency.
func EncodeULID(s string) (string, error) {
	if len(s) != 26 {
		return "", fmt.Errorf("ddbgen: ulid encoder requires 26 characters, got %d", len(s))
	}
	u := strings.ToUpper(s)
	if u[0] > '7' {
		return "", fmt.Errorf("ddbgen: invalid ulid %q: first character must be 0-7", s)
	}
	for i := 0; i < len(u); i++ {
		if !strings.ContainsRune(crockford, rune(u[i])) {
			return "", fmt.Errorf("ddbgen: invalid ulid %q: character %q is not Crockford base32", s, u[i])
		}
	}
	return u, nil
}

// DecodeULID is the identity inverse of EncodeULID (the stored form is the
// canonical uppercase ULID).
func DecodeULID(s string) (string, error) {
	return EncodeULID(s)
}

// EncodeURL escapes a string so it cannot contain the key delimiter. This
// is the escape hatch for values that legitimately contain the delimiter.
func EncodeURL(s string) string {
	return url.QueryEscape(s)
}

// DecodeURL is the inverse of EncodeURL.
func DecodeURL(s string) (string, error) {
	v, err := url.QueryUnescape(s)
	if err != nil {
		return "", fmt.Errorf("ddbgen: decoding urlenc key segment: %w", err)
	}
	return v, nil
}
