package tui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// omnibox is the ':' go-to bar (8000_ant_tui §7.5): a single text input that
// runs whatever you type through Engine.Resolve, so a bare id, a handle, a live
// URL, or a full resource URI all land on a record. It is owned by the App and
// overlays the status line while active; submitting emits a resolveCmd whose
// result the App turns into navigation.
type omnibox struct {
	d      *deps
	ti     textinput.Model
	active bool
}

func newOmnibox(d *deps) omnibox {
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetVirtualCursor(true)
	ti.Placeholder = "scheme://type/id, a handle, or a live URL"
	return omnibox{d: d, ti: ti}
}

// open focuses the bar, seeding it with the current subject so refining an
// address is a small edit rather than retyping.
func (o *omnibox) open(seed string) tea.Cmd {
	o.active = true
	o.ti.SetValue(seed)
	o.ti.CursorEnd()
	return o.ti.Focus()
}

func (o *omnibox) close() {
	o.active = false
	o.ti.Blur()
	o.ti.Reset()
}

func (o *omnibox) setWidth(w int) {
	if w > 4 {
		o.ti.SetWidth(w - 4)
	}
}

// update routes a key to the bar. Enter resolves the input and closes; Esc
// cancels; everything else edits.
func (o omnibox) update(msg tea.Msg) (omnibox, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		o.ti, cmd = o.ti.Update(msg)
		return o, cmd
	}
	switch {
	case key.Matches(kp, o.d.keys.Escape):
		o.close()
		return o, nil
	case key.Matches(kp, o.d.keys.Enter):
		input := strings.TrimSpace(o.ti.Value())
		o.close()
		if input == "" {
			return o, nil
		}
		return o, o.submit(input)
	default:
		var cmd tea.Cmd
		o.ti, cmd = o.ti.Update(msg)
		return o, cmd
	}
}

// submit interprets the bar. A leading verb (browse/domain/search/ls/graph/
// links) opens that screen directly; anything else is run through Resolve, so a
// bare id, a handle, a live URL, or a full URI all land on a record
// (8000_ant_tui §7.6). The verbs that act on a record resolve their target
// inline, since Resolve is a synchronous, offline lookup.
func (o omnibox) submit(input string) tea.Cmd {
	verb, rest, _ := strings.Cut(input, " ")
	rest = strings.TrimSpace(rest)
	switch verb {
	case "browse":
		return push(newBrowseScreen(o.d, rest))
	case "domain":
		if rest == "" {
			return toastCmd("domain: need a scheme", true)
		}
		return push(newDomainScreen(o.d, rest))
	case "search":
		scheme, q, _ := strings.Cut(rest, " ")
		if scheme == "" {
			return toastCmd("search: need a scheme", true)
		}
		return push(newSearchScreenWith(o.d, scheme, q))
	case "ls", "graph", "links":
		return o.recordVerb(verb, rest)
	default:
		return resolveCmd(o.d.e, input, "")
	}
}

// recordVerb opens a record-scoped screen (ls/graph/links) by resolving the rest
// of the line to a URI first. graph accepts a trailing depth.
func (o omnibox) recordVerb(verb, rest string) tea.Cmd {
	depth := 1
	target := rest
	if verb == "graph" {
		if head, tail, ok := strings.Cut(rest, " "); ok {
			if d, err := strconv.Atoi(strings.TrimSpace(tail)); err == nil {
				depth, target = d, head
			}
		}
	}
	if strings.TrimSpace(target) == "" {
		return toastCmd(verb+": need a record", true)
	}
	u, err := o.d.e.Resolve(target, "")
	if err != nil {
		return toastCmd(verb+": "+err.Error(), true)
	}
	switch verb {
	case "ls":
		return push(newCollectionScreen(o.d, u))
	case "graph":
		return push(newGraphScreen(o.d, u, depth))
	case "links":
		return push(newLinksScreen(o.d, u))
	}
	return nil
}

func (o omnibox) View(sty *styles) string {
	return sty.Title.Render("go to ") + sty.Crumb.Render("› ") + o.ti.View()
}
