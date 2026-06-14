package tui

import (
	"bytes"
	"encoding/json"
	"strings"
)

// renderJSON pretty-prints an envelope's raw JSON for the raw view, lightly
// coloring object keys so the structure reads at a glance (8000_ant_tui §8.4).
// It never alters the bytes' meaning: a failed re-indent falls back to the raw
// text, so the raw view always shows exactly what the JSON API would return.
func renderJSON(raw []byte, sty *styles) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return sty.Base.Render(string(raw))
	}
	lines := strings.Split(buf.String(), "\n")
	for i, ln := range lines {
		lines[i] = colorJSONLine(ln, sty)
	}
	return strings.Join(lines, "\n")
}

// colorJSONLine dims the object key at the head of a line (the text up to the
// first `":`), leaving the value in the base color.
func colorJSONLine(ln string, sty *styles) string {
	trimmed := strings.TrimLeft(ln, " ")
	indent := ln[:len(ln)-len(trimmed)]
	if !strings.HasPrefix(trimmed, `"`) {
		return sty.Base.Render(ln)
	}
	// Find the closing quote of the key, then the colon.
	end := strings.Index(trimmed[1:], `"`)
	if end < 0 {
		return sty.Base.Render(ln)
	}
	keyEnd := end + 2 // include both quotes
	rest := trimmed[keyEnd:]
	if !strings.HasPrefix(strings.TrimLeft(rest, " "), ":") {
		return sty.Base.Render(ln)
	}
	key := trimmed[:keyEnd]
	return indent + sty.Key.Render(key) + sty.Base.Render(rest)
}
