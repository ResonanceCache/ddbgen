package codegen

import (
	"go/token"
	"strings"
	"unicode"
)

// lowerCamel converts an exported Go identifier to a parameter name:
// TenantID -> tenantID, ID -> id, URLPath -> urlPath, CreatedAt -> createdAt.
func lowerCamel(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	// Length of the leading uppercase run.
	n := 0
	for n < len(runes) && unicode.IsUpper(runes[n]) {
		n++
	}
	switch {
	case n == 0:
		// already lower-cased
	case n == len(runes):
		// whole identifier is one acronym: ID -> id
		for i := 0; i < n; i++ {
			runes[i] = unicode.ToLower(runes[i])
		}
	case n == 1:
		runes[0] = unicode.ToLower(runes[0])
	default:
		// leading acronym followed by a word: URLPath -> urlPath. The last
		// upper rune starts the next word and keeps its case.
		for i := 0; i < n-1; i++ {
			runes[i] = unicode.ToLower(runes[i])
		}
	}
	return safeIdent(string(runes))
}

// reservedIdents are identifiers generated code uses for receivers, common
// locals, and imported package names; parameters must not shadow them.
var reservedIdents = map[string]bool{
	"c": true, "u": true, "q": true, "v": true, "k": true, "ctx": true,
	"err": true, "uerr": true, "av": true, "out": true, "pk": true,
	"sk": true, "cond": true, "names": true, "values": true,
	"expected": true, "input": true, "pred": true, "ok": true, "enc": true,
	"encLo": true, "encHi": true, "lo": true, "hi": true, "val": true,
	"delta": true, "items": true, "keys": true, "kk": true, "raw": true,
	"col": true, "spec": true, "do": true, "it": true, "dup": true,
	"aws": true, "attributevalue": true, "dynamodb": true, "types": true,
	"runtime": true, "fmt": true, "time": true, "strings": true,
	"strconv": true, "context": true, "iter": true, "errors": true,
}

func safeIdent(s string) string {
	if token.IsKeyword(s) || reservedIdents[s] {
		return s + "Arg"
	}
	return s
}

// exported upper-cases the first rune: app -> App.
func exported(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// snake converts CamelCase to snake_case for file names:
// OrderLine -> order_line, HTTPServer -> http_server.
func snake(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			prevLower := i > 0 && !unicode.IsUpper(runes[i-1])
			nextLower := i+1 < len(runes) && !unicode.IsUpper(runes[i+1]) && unicode.IsLetter(runes[i+1])
			if i > 0 && (prevLower || nextLower) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
