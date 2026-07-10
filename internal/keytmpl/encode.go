package keytmpl

import (
	"fmt"
	"strings"
	"time"

	"github.com/ResonanceCache/ddbgen/runtime"
)

// Encode renders the template with the given placeholder values, applying
// each placeholder's encoder. Values are keyed by field name. This is the
// generator-side engine; generated code inlines equivalent runtime calls.
func (t *Template) Encode(vals map[string]any) (string, error) {
	var b strings.Builder
	for i, seg := range t.Segments {
		if i > 0 {
			b.WriteString(Delimiter)
		}
		if seg.Kind == SegLiteral {
			b.WriteString(seg.Literal)
			continue
		}
		v, ok := vals[seg.Field]
		if !ok {
			return "", fmt.Errorf("missing value for placeholder {%s}", seg.Field)
		}
		enc, err := EncodeSegment(seg, v)
		if err != nil {
			return "", fmt.Errorf("encoding {%s}: %w", seg.Field, err)
		}
		b.WriteString(enc)
	}
	return b.String(), nil
}

// EncodeSegment encodes a single placeholder value.
func EncodeSegment(seg Segment, v any) (string, error) {
	switch seg.Encoder {
	case "":
		s, err := asString(v)
		if err != nil {
			return "", err
		}
		return runtime.CheckSegment(s)
	case "rfc3339":
		t, ok := v.(time.Time)
		if !ok {
			return "", typeErr(seg, v, "time.Time")
		}
		return runtime.EncodeRFC3339(t)
	case "epoch":
		if t, ok := v.(time.Time); ok {
			return runtime.EncodeEpochTime(t)
		}
		n, err := asInt64(v)
		if err != nil {
			return "", err
		}
		return runtime.EncodeEpoch(n)
	case "epochms":
		if t, ok := v.(time.Time); ok {
			return runtime.EncodeEpochMSTime(t)
		}
		n, err := asInt64(v)
		if err != nil {
			return "", err
		}
		return runtime.EncodeEpochMS(n)
	case "upper":
		s, err := asString(v)
		if err != nil {
			return "", err
		}
		return runtime.CheckSegment(strings.ToUpper(s))
	case "lower":
		s, err := asString(v)
		if err != nil {
			return "", err
		}
		return runtime.CheckSegment(strings.ToLower(s))
	case "hex":
		b, ok := v.([]byte)
		if !ok {
			return "", typeErr(seg, v, "[]byte")
		}
		return runtime.EncodeHex(b), nil
	case "ulid":
		s, err := asString(v)
		if err != nil {
			return "", err
		}
		return runtime.EncodeULID(s)
	case "urlenc":
		s, err := asString(v)
		if err != nil {
			return "", err
		}
		return runtime.EncodeURL(s), nil
	}
	if w := PadWidth(seg.Encoder); w > 0 {
		if u, ok := v.(uint64); ok {
			return runtime.EncodePadUint(u, w)
		}
		n, err := asInt64(v)
		if err != nil {
			return "", err
		}
		return runtime.EncodePad(n, w)
	}
	return "", fmt.Errorf("unknown encoder %q", seg.Encoder)
}

// Decode splits an encoded key and inverts each placeholder's encoder,
// returning canonical Go values: string for raw/upper/lower/ulid/urlenc,
// time.Time for rfc3339, int64 for epoch/epochms/pad<N>, []byte for hex.
func (t *Template) Decode(key string) (map[string]any, error) {
	parts := strings.Split(key, Delimiter)
	if len(parts) != len(t.Segments) {
		return nil, fmt.Errorf("key %q has %d segments; template %q has %d", key, len(parts), t.Raw, len(t.Segments))
	}
	out := map[string]any{}
	for i, seg := range t.Segments {
		if seg.Kind == SegLiteral {
			if parts[i] != seg.Literal {
				return nil, fmt.Errorf("key %q: segment %d is %q, want literal %q", key, i, parts[i], seg.Literal)
			}
			continue
		}
		v, err := DecodeSegment(seg, parts[i])
		if err != nil {
			return nil, fmt.Errorf("decoding {%s} from %q: %w", seg.Field, parts[i], err)
		}
		out[seg.Field] = v
	}
	return out, nil
}

// DecodeSegment inverts a single placeholder encoding.
func DecodeSegment(seg Segment, s string) (any, error) {
	switch seg.Encoder {
	case "", "upper", "lower":
		// upper/lower are normalizing: the stored form is canonical.
		return s, nil
	case "rfc3339":
		return runtime.DecodeRFC3339(s)
	case "epoch":
		return runtime.DecodeEpoch(s)
	case "epochms":
		return runtime.DecodeEpochMS(s)
	case "hex":
		return runtime.DecodeHex(s)
	case "ulid":
		return runtime.DecodeULID(s)
	case "urlenc":
		return runtime.DecodeURL(s)
	}
	if w := PadWidth(seg.Encoder); w > 0 {
		return runtime.DecodePad(s, w)
	}
	return nil, fmt.Errorf("unknown encoder %q", seg.Encoder)
}

func asString(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("want string, got %T", v)
	}
	return s, nil
}

func asInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case int32:
		return int64(n), nil
	case uint32:
		return int64(n), nil
	}
	return 0, fmt.Errorf("want integer, got %T", v)
}

func typeErr(seg Segment, v any, want string) error {
	return fmt.Errorf("encoder %s wants %s, got %T", seg.Encoder, want, v)
}
