package tui

import "charm.land/bubbles/v2/key"

// keyMap is the whole program's keymap in one value (8000_ant_tui §10). It is
// synthesized from the conventions of k9s, lazygit, gh-dash, glow and ranger:
// vim motion, ':' for the omnibox, Esc/Backspace for back, '?' for help. Screens
// expose the subset they honor through ShortHelp/FullHelp so the footer and the
// help overlay stay truthful per screen.
type keyMap struct {
	// motion
	Up       key.Binding
	Down     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	HalfUp   key.Binding
	HalfDown key.Binding
	Left     key.Binding
	Right    key.Binding

	// navigation
	Enter   key.Binding
	Back    key.Binding
	Forward key.Binding
	Home    key.Binding

	// actions on the current record
	Refresh key.Binding
	Links   key.Binding
	List    key.Binding
	Graph   key.Binding
	URL     key.Binding
	Copy    key.Binding
	Body    key.Binding
	Mode    key.Binding
	Export  key.Binding
	Browse  key.Binding
	Tab     key.Binding

	// modal / global
	Omni    key.Binding
	Filter  key.Binding
	Domains key.Binding
	Help    key.Binding
	Theme   key.Binding
	Quit    key.Binding
	Escape  key.Binding
}

func newKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		HalfUp:   key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u", "half page up")),
		HalfDown: key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d", "half page down")),
		Left:     key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "left")),
		Right:    key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "right")),

		Enter:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:    key.NewBinding(key.WithKeys("esc", "backspace"), key.WithHelp("esc", "back")),
		Forward: key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "forward")),
		Home:    key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "home")),

		Refresh: key.NewBinding(key.WithKeys("r", "ctrl+r"), key.WithHelp("r", "refresh")),
		Links:   key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "links")),
		List:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "list")),
		Graph:   key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "graph")),
		URL:     key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "live url")),
		Copy:    key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy uri")),
		Body:    key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "body")),
		Mode:    key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "view: data/body/raw")),
		Export:  key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "export")),
		Browse:  key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "browse")),
		Tab:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "focus")),

		Omni:    key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "go to")),
		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Domains: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "domains")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Theme:   key.NewBinding(key.WithKeys("T"), key.WithHelp("T", "theme")),
		Quit:    key.NewBinding(key.WithKeys("ctrl+c", "q"), key.WithHelp("q", "quit")),
		Escape:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}
