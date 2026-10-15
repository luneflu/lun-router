package compactor

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// JSONMinTableRows is the minimum length of a uniform object array before it is
// folded into the fields/rows table form.
const JSONMinTableRows = 20

// IsJSONLike reports whether text is a JSON object or array (not a bare scalar).
func IsJSONLike(text string) bool {
	t := strings.TrimSpace(text)
	if len(t) < 2 || (t[0] != '{' && t[0] != '[') {
		return false
	}
	return json.Valid([]byte(t))
}

// CompressJSON rewrites JSON tool output losslessly: minify, then, for uniform
// arrays of objects, fold into {"_compacted":"table","fields":[...],"rows":[[...]]}.
// No row and no field is ever dropped, so the result is equivalent to the input.
// ok is false when nothing was saved.
func CompressJSON(text string) (string, bool) {
	if !IsJSONLike(text) {
		return text, false
	}

	var v any
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return text, false
	}

	minified, err := encodeJSON(v)
	if err != nil || len(minified) >= len(text) {
		return text, false
	}

	if table, ok := foldTable(v); ok && len(table) < len(minified) {
		return table, true
	}
	return minified, true
}

// foldTable encodes a uniform array of objects as a fields list plus one value
// array per row. Any key mismatch or non-object element aborts the fold.
func foldTable(v any) (string, bool) {
	arr, ok := v.([]any)
	if !ok || len(arr) < JSONMinTableRows {
		return "", false
	}

	var fields []string
	for i, item := range arr {
		obj, ok := item.(map[string]any)
		if !ok {
			return "", false
		}
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if i == 0 {
			fields = keys
			continue
		}
		if !equalStrings(fields, keys) {
			return "", false
		}
	}

	rows := make([][]any, 0, len(arr))
	for _, item := range arr {
		obj := item.(map[string]any)
		row := make([]any, 0, len(fields))
		for _, f := range fields {
			row = append(row, obj[f])
		}
		rows = append(rows, row)
	}

	out, err := encodeJSON(map[string]any{"_compacted": "table", "fields": fields, "rows": rows})
	if err != nil {
		return "", false
	}
	return out, true
}

func encodeJSON(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
