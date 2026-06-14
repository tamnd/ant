package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// renderHelp draws the full-screen help overlay: the active screen's bindings
// grouped into columns, plus the always-available global keys (8000_ant_tui §10).
// It is a pure function of the bindings so it needs no model of its own; the App
// flips a bool to show it.
func renderHelp(sty *styles, w, h int, groups [][]key.Binding) string {
	var cols []string
	for _, g := range groups {
		cols = append(cols, helpColumn(sty, g))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, spaced(cols, "   ")...)
	title := sty.Title.Render("ant tui — keys")
	footer := sty.Muted.Render("press ? or esc to close")
	block := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", footer)
	return padLines(block, w, h)
}

// helpColumn renders one group of bindings as aligned key/desc rows.
func helpColumn(sty *styles, bs []key.Binding) string {
	keyW := 0
	for _, b := range bs {
		if !b.Enabled() {
			continue
		}
		if w := lipgloss.Width(b.Help().Key); w > keyW {
			keyW = w
		}
	}
	var rows []string
	for _, b := range bs {
		if !b.Enabled() {
			continue
		}
		k := sty.Title.Width(keyW).Render(b.Help().Key)
		rows = append(rows, k+"  "+sty.Muted.Render(b.Help().Desc))
	}
	return strings.Join(rows, "\n")
}

// globalHelp is the set of keys every screen honors, shown in its own help column.
func globalHelp(keys keyMap) []key.Binding {
	return []key.Binding{
		keys.Omni, keys.Back, keys.Forward, keys.Home,
		keys.Domains, keys.Theme, keys.Help, keys.Quit,
	}
}

// spaced interleaves a separator column between rendered columns.
func spaced(cols []string, sep string) []string {
	if len(cols) == 0 {
		return cols
	}
	out := make([]string, 0, len(cols)*2-1)
	for i, c := range cols {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, c)
	}
	return out
}
