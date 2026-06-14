package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// searchListN is how many hits a search asks for.
const searchListN = 25

// searchScreen runs a domain's free-text search (ant search --on <scheme>): a
// query field on top, followable results below. While the field is focused the
// screen Captures keys, so typing a query never trips the global keymap
// (8000_ant_tui §7.5, §10.3).
type searchScreen struct {
	screenBase
	scheme   string
	query    string
	ti       textinput.Model
	pick     picker
	editing  bool
	inflight bool
	err      error
	searched bool
	pending  string // a query to run on Init (from the omnibox :search verb)
}

func newSearchScreen(d *deps, scheme string) *searchScreen {
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetVirtualCursor(true)
	ti.Placeholder = "type a query, enter to run"
	s := &searchScreen{screenBase: screenBase{d: d}, scheme: scheme, ti: ti, pick: newPicker()}
	s.pick.empty = "No results yet."
	return s
}

// newSearchScreenWith opens search already aimed at a query, for the omnibox
// ':search <scheme> <query>' verb: it runs the query on Init instead of waiting
// for the field.
func newSearchScreenWith(d *deps, scheme, query string) *searchScreen {
	s := newSearchScreen(d, scheme)
	if q := strings.TrimSpace(query); q != "" {
		s.ti.SetValue(q)
		s.pending = q
	}
	return s
}

func (s *searchScreen) Init() tea.Cmd {
	if s.pending != "" {
		q := s.pending
		s.pending = ""
		s.query, s.inflight, s.searched, s.err = q, true, true, nil
		return searchCmd(s.d.ctx, s.d.e, s.scheme, q, searchListN)
	}
	s.editing = true
	return s.ti.Focus()
}

func (s *searchScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = msg.Width, msg.Height
		if s.w > 4 {
			s.ti.SetWidth(s.w - 4)
		}
	case searchedMsg:
		if msg.Key != searchKey(s.scheme, searchListN, s.query) {
			return s, nil
		}
		s.inflight = false
		if msg.Err != nil {
			s.err = msg.Err
			return s, nil
		}
		s.pick.setItems(envItems(msg.Envs))
	case tea.KeyPressMsg:
		return s.onKey(msg)
	}
	return s, nil
}

func (s *searchScreen) onKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	k := s.d.keys
	if s.editing {
		switch {
		case key.Matches(msg, k.Escape):
			s.editing = false
			s.ti.Blur()
			return s, nil
		case key.Matches(msg, k.Enter):
			q := s.ti.Value()
			s.editing = false
			s.ti.Blur()
			if q == "" {
				return s, nil
			}
			s.query, s.inflight, s.searched, s.err = q, true, true, nil
			return s, searchCmd(s.d.ctx, s.d.e, s.scheme, q, searchListN)
		default:
			var cmd tea.Cmd
			s.ti, cmd = s.ti.Update(msg)
			return s, cmd
		}
	}
	switch {
	case key.Matches(msg, k.Filter):
		s.editing = true
		return s, s.ti.Focus()
	case key.Matches(msg, k.Enter):
		if it, ok := s.pick.selected(); ok && it.hasURI {
			return s, navigate(it.uri)
		}
	}
	s.pick.handle(msg, k)
	return s, nil
}

func (s *searchScreen) View(w, h int) string {
	sty := s.d.sty
	label := sty.Title.Render("search "+s.scheme) + sty.Crumb.Render(" › ")
	field := label + s.ti.View()
	var status string
	switch {
	case s.err != nil:
		status = sty.Err.Render(s.err.Error())
	case s.inflight:
		status = sty.Muted.Render("searching …")
	case s.searched:
		status = sty.Muted.Render("enter to open · / to edit query")
	default:
		status = sty.Muted.Render("type a query and press enter")
	}
	s.pick.setSize(w, max(1, h-3))
	return field + "\n" + status + "\n" + s.pick.View(sty)
}

func (s *searchScreen) Title() string   { return "search " + s.scheme }
func (s *searchScreen) Capturing() bool { return s.editing }
func (s *searchScreen) Loading() bool   { return s.inflight }
func (s *searchScreen) ShortHelp() []key.Binding {
	k := s.d.keys
	return []key.Binding{k.Filter, k.Enter, k.Down, k.Back}
}
func (s *searchScreen) FullHelp() [][]key.Binding {
	k := s.d.keys
	return [][]key.Binding{
		{k.Filter, k.Enter},
		{k.Up, k.Down, k.Top, k.Bottom, k.Back},
	}
}
