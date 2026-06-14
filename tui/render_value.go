package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/tamnd/any-cli/kit"
)

// This file ports the web console's order-preserving value renderer
// (web/render.go) to the terminal: the same decode tree and the same field
// shaping, but emitting lipgloss-styled text instead of HTML. Sharing the shape
// keeps the two surfaces showing a record the same way (8000_ant_tui §8).

// kvPair preserves a record object's declared field order (a map would lose it).
type kvPair struct {
	Key string
	Val any
}

type orderedObj []kvPair

// decodeOrdered reads JSON into an order-preserving tree: objects become
// orderedObj, arrays []any, numbers json.Number, the rest their natural types.
func decodeOrdered(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch delim {
	case '{':
		obj := orderedObj{}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			val, err := decodeOrdered(dec)
			if err != nil {
				return nil, err
			}
			obj = append(obj, kvPair{Key: keyTok.(string), Val: val})
		}
		_, err = dec.Token() // closing '}'
		return obj, err
	case '[':
		arr := []any{}
		for dec.More() {
			val, err := decodeOrdered(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		_, err = dec.Token() // closing ']'
		return arr, err
	default:
		return nil, fmt.Errorf("unexpected delimiter %v", delim)
	}
}

// orderedDataFromRaw pulls the "data" object out of an envelope's raw JSON,
// preserving the record's declared field order, so a cache-loaded record shows
// its fields in the same order a freshly fetched one does (8000_ant_tui §8.2).
func orderedDataFromRaw(raw []byte) orderedObj {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := decodeOrdered(dec)
	if err != nil {
		return nil
	}
	env, ok := v.(orderedObj)
	if !ok {
		return nil
	}
	for _, kv := range env {
		if kv.Key == "data" {
			if d, ok := kv.Val.(orderedObj); ok {
				return d
			}
			return nil
		}
	}
	return nil
}

// renderData renders a record's data object as an aligned key/value block inside
// the given content width. URI-valued fields are accented and arrow-marked so a
// reader can see at a glance which fields are followable edges (8000_ant_tui §8.1).
func renderData(raw []byte, sty *styles, width int) string {
	data := orderedDataFromRaw(raw)
	if len(data) == 0 {
		return sty.Muted.Render("(no fields)")
	}
	keyW := 0
	for _, kv := range data {
		if w := lipgloss.Width(humanize(kv.Key)); w > keyW {
			keyW = w
		}
	}
	if keyW > 22 {
		keyW = 22
	}
	valW := width - keyW - 1
	if valW < 8 {
		valW = 8
	}
	var b strings.Builder
	for i, kv := range data {
		if i > 0 {
			b.WriteByte('\n')
		}
		key := sty.Key.Width(keyW).Render(humanize(kv.Key))
		val := valueString(kv.Key, kv.Val, sty, valW, 0)
		b.WriteString(key + " " + indentAfterFirst(val, keyW+1))
	}
	return b.String()
}

// valueString renders one record value to a possibly multi-line styled string,
// the terminal analogue of valueHTML.
func valueString(field string, v any, sty *styles, width, depth int) string {
	if depth > 8 {
		return sty.Muted.Render("…")
	}
	switch x := v.(type) {
	case nil:
		return sty.Muted.Render("—")
	case string:
		return stringValue(field, x, sty, width)
	case bool:
		return sty.Base.Render(strconv.FormatBool(x))
	case json.Number:
		return sty.Base.Render(groupNumber(x.String()))
	case float64:
		return sty.Base.Render(groupNumber(strconv.FormatFloat(x, 'f', -1, 64)))
	case orderedObj:
		if len(x) == 0 {
			return sty.Muted.Render("{}")
		}
		var lines []string
		for _, kv := range x {
			val := oneLine(valueString(kv.Key, kv.Val, sty, width, depth+1))
			lines = append(lines, sty.Key.Render(humanize(kv.Key)+": ")+val)
		}
		return strings.Join(lines, "\n")
	case []any:
		if len(x) == 0 {
			return sty.Muted.Render("[]")
		}
		var lines []string
		for _, item := range x {
			lines = append(lines, valueString(field, item, sty, width, depth+1))
		}
		return strings.Join(lines, "\n")
	default:
		return sty.Base.Render(fmt.Sprint(x))
	}
}

// stringValue styles a string: a resource URI becomes an accent-colored, arrow-
// marked chip; any other URL is dimmed and arrow-marked outward; plain text is
// shown and clamped to the width.
func stringValue(field, s string, sty *styles, width int) string {
	if u, err := kit.ParseURI(s); err == nil && !kit.IsReservedKind(u.Scheme) {
		label := u.Authority + "/" + u.ID()
		return sty.schemeStyle(u.Scheme).Render("→ " + label)
	}
	if isHTTPURL(s) {
		w := width - 2
		if w < 12 {
			w = 12
		}
		return sty.Muted.Render(truncate(s, w) + " ↗")
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if width > 4 && lipgloss.Width(s) > width {
		s = truncate(s, width)
	}
	return sty.Base.Render(s)
}

// linkRow is one followable edge in a record's @links, flattened from the grouped
// map into the order the links list and the resource pane render.
type linkRow struct {
	Field string
	URI   kit.URI
	Raw   string
}

// flattenLinks turns an envelope's grouped @links into a stable, sorted-by-field
// slice of followable rows, dropping any that fail to parse.
func flattenLinks(m map[string][]string) []linkRow {
	if len(m) == 0 {
		return nil
	}
	fields := make([]string, 0, len(m))
	for f := range m {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	var out []linkRow
	for _, f := range fields {
		for _, raw := range m[f] {
			u, err := kit.ParseURI(raw)
			if err != nil {
				continue
			}
			out = append(out, linkRow{Field: f, URI: u, Raw: raw})
		}
	}
	return out
}

// --- small helpers (ported from web/render.go) ------------------------------

// humanize turns a json field name into a label: similar_books -> "Similar books".
func humanize(name string) string {
	if name == "" {
		return name
	}
	s := strings.NewReplacer("_", " ", "-", " ").Replace(name)
	return strings.ToUpper(s[:1]) + s[1:]
}

// relTime renders an RFC3339 timestamp as a relative string; non-times pass back.
func relTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("2006-01-02 15:04")
	}
}

// shortID is the last path segment of a URI, for compact graph/card labels.
func shortID(uri string) string {
	u, err := kit.ParseURI(uri)
	if err != nil {
		return uri
	}
	if len(u.Path) == 0 {
		return u.Authority
	}
	return u.Path[len(u.Path)-1]
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// groupNumber inserts thousands separators into a plain integer string; it
// leaves decimals and non-numbers untouched.
func groupNumber(s string) string {
	neg := strings.HasPrefix(s, "-")
	d := strings.TrimPrefix(s, "-")
	if d == "" || strings.ContainsAny(d, ".eE") {
		return s
	}
	for _, r := range d {
		if r < '0' || r > '9' {
			return s
		}
	}
	if len(d) <= 4 {
		return s
	}
	var b strings.Builder
	for i, r := range d {
		if i > 0 && (len(d)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// truncate clamps s to n runes with an ellipsis, measuring by rune so a multibyte
// string is not cut mid-character.
func truncate(s string, n int) string {
	if n <= 1 {
		return "…"
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// oneLine collapses a multi-line rendering to a single line, for nested-object
// inline display.
func oneLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}

// indentAfterFirst left-pads every line after the first by n spaces, so a
// multi-line value aligns under the value column rather than the key.
func indentAfterFirst(s string, n int) string {
	if !strings.Contains(s, "\n") {
		return s
	}
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}
