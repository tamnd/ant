package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tamnd/any-cli/kit"
)

// pickItem is one row in a picker: a title, an optional dim subtitle, and the URI
// it opens (if any). The scheme drives the leading accent dot.
type pickItem struct {
	title    string
	subtitle string
	scheme   string
	uri      kit.URI
	hasURI   bool
	// payload carries screen-specific data a row may need on Enter (a browse
	// prefix, a domain scheme) without a parallel slice.
	payload string
}

// picker is the program's one list widget: a windowed, cursor-driven column used
// by every collection-shaped screen (links, ls, search, graph, browse, domains).
// Rolling our own keeps full control of the row rendering (accent dots, dim
// subtitles) and makes the screens trivial to unit-test, since selection is plain
// state rather than a delegate (8000_ant_tui §7.3, §3.3).
type picker struct {
	items  []pickItem
	cursor int
	off    int
	w, h   int
	empty  string
}

func newPicker() picker { return picker{empty: "Nothing here."} }

func (p *picker) setItems(items []pickItem) {
	p.items = items
	if p.cursor >= len(items) {
		p.cursor = max(0, len(items)-1)
	}
	p.clamp()
}

func (p *picker) setSize(w, h int) {
	p.w, p.h = w, h
	p.clamp()
}

func (p *picker) up()     { p.move(-1) }
func (p *picker) down()   { p.move(1) }
func (p *picker) top()    { p.cursor = 0; p.clamp() }
func (p *picker) bottom() { p.cursor = max(0, len(p.items)-1); p.clamp() }

func (p *picker) move(d int) {
	if len(p.items) == 0 {
		return
	}
	p.cursor += d
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.items) {
		p.cursor = len(p.items) - 1
	}
	p.clamp()
}

// clamp keeps the cursor inside the visible window, scrolling the window to
// follow it.
func (p *picker) clamp() {
	if p.h <= 0 {
		return
	}
	if p.cursor < p.off {
		p.off = p.cursor
	}
	if p.cursor >= p.off+p.h {
		p.off = p.cursor - p.h + 1
	}
	if p.off < 0 {
		p.off = 0
	}
}

func (p *picker) selected() (pickItem, bool) {
	if p.cursor < 0 || p.cursor >= len(p.items) {
		return pickItem{}, false
	}
	return p.items[p.cursor], true
}

// handle consumes the motion keys a picker owns and reports whether it did, so a
// screen can fall through to its own keys for anything else.
func (p *picker) handle(msg tea.KeyPressMsg, keys keyMap) bool {
	switch {
	case key.Matches(msg, keys.Up):
		p.up()
	case key.Matches(msg, keys.Down):
		p.down()
	case key.Matches(msg, keys.Top):
		p.top()
	case key.Matches(msg, keys.Bottom):
		p.bottom()
	case key.Matches(msg, keys.HalfDown):
		for i := 0; i < p.h/2; i++ {
			p.down()
		}
	case key.Matches(msg, keys.HalfUp):
		for i := 0; i < p.h/2; i++ {
			p.up()
		}
	default:
		return false
	}
	return true
}

// View renders exactly h lines (padding with blanks) so the screen layout below
// it stays put as the list shortens.
func (p picker) View(sty *styles) string {
	if len(p.items) == 0 {
		return padLines(sty.Muted.Render(p.empty), p.w, p.h)
	}
	var b strings.Builder
	end := min(p.off+p.h, len(p.items))
	for i := p.off; i < end; i++ {
		if i > p.off {
			b.WriteByte('\n')
		}
		b.WriteString(p.line(sty, i, i == p.cursor))
	}
	return padLines(b.String(), p.w, p.h)
}

func (p picker) line(sty *styles, i int, sel bool) string {
	it := p.items[i]
	dot := "  "
	if it.scheme != "" {
		dot = sty.schemeStyle(it.scheme).Render("● ")
	}
	title := it.title
	line := dot + title
	if it.subtitle != "" {
		line += "  " + sty.Muted.Render(it.subtitle)
	}
	line = clampLine(line, p.w)
	if sel {
		return sty.Sel.Width(p.w).Render(stripToWidth(line, p.w))
	}
	return line
}

// envItems turns a slice of envelopes (ls members, search hits) into followable
// picker rows, labeled by record with the type as the dim subtitle.
func envItems(envs []kit.Envelope) []pickItem {
	items := make([]pickItem, 0, len(envs))
	for _, e := range envs {
		u, err := kit.ParseURI(e.ID)
		if err != nil {
			items = append(items, pickItem{title: e.ID, subtitle: e.Type, scheme: schemeOf(e.Type)})
			continue
		}
		items = append(items, pickItem{
			title:    u.Authority + "/" + u.ID(),
			subtitle: e.Type,
			scheme:   u.Scheme,
			uri:      u,
			hasURI:   true,
		})
	}
	return items
}

// --- layout helpers ---------------------------------------------------------

// padLines pads a block to exactly h lines and clamps each to w, so panes keep a
// fixed footprint regardless of content.
func padLines(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if h > 0 {
		for len(lines) < h {
			lines = append(lines, "")
		}
		if len(lines) > h {
			lines = lines[:h]
		}
	}
	if w > 0 {
		for i := range lines {
			lines[i] = clampLine(lines[i], w)
		}
	}
	return strings.Join(lines, "\n")
}

// clampLine truncates a single (possibly styled) line to a printable width w,
// leaving shorter lines untouched.
func clampLine(s string, w int) string {
	if w <= 0 || lipgloss.Width(s) <= w {
		return s
	}
	return truncateANSI(s, w)
}

// stripToWidth returns s clamped to w but without truncating styled-width math;
// used before applying a full-width background so the highlight is exactly w wide.
func stripToWidth(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return truncateANSI(s, w)
}

// truncateANSI clamps a styled string to width w by measuring visible cells and
// re-truncating on the plain text when it overflows. lipgloss styles applied to
// the result keep their codes; for our single-style lines this is sufficient and
// avoids a heavyweight ANSI-aware truncator.
func truncateANSI(s string, w int) string {
	if w <= 1 {
		return ""
	}
	// Fast path: no escape codes.
	if !strings.Contains(s, "\x1b") {
		return truncate(s, w)
	}
	// Styled: fall back to lipgloss width-aware clamp via a max-width style.
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}
