package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/tamnd/any-cli/kit"
)

// linksScreen is the full-screen view of a record's typed outbound edges (ant
// links): every @link as a followable row, grouped by the field it came from.
// It reads the edges straight off the cached envelope, painting instantly, and
// fills in from a background Dereference only on a cold record (8000_ant_tui §7.4).
type linksScreen struct {
	screenBase
	u        kit.URI
	pick     picker
	inflight bool
	err      error
}

func newLinksScreen(d *deps, u kit.URI) *linksScreen {
	s := &linksScreen{screenBase: screenBase{d: d}, u: u, pick: newPicker()}
	s.pick.empty = "No outbound links."
	return s
}

func (s *linksScreen) Init() tea.Cmd {
	if f, ok := s.d.e.Lookup(s.u); ok {
		s.pick.setItems(linkItemsFrom(f.Env.Links))
		return nil
	}
	s.inflight = true
	return derefCmd(s.d.ctx, s.d.e, s.u, false)
}

func (s *linksScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = msg.Width, msg.Height
	case fetchedMsg:
		if msg.URI.String() != s.u.String() {
			return s, nil
		}
		s.inflight = false
		if msg.Err != nil {
			s.err = msg.Err
			return s, nil
		}
		s.pick.setItems(linkItemsFrom(msg.Fetched.Env.Links))
	case tea.KeyPressMsg:
		if s.pick.handle(msg, s.d.keys) {
			return s, nil
		}
		if key.Matches(msg, s.d.keys.Enter) {
			if it, ok := s.pick.selected(); ok && it.hasURI {
				return s, navigate(it.uri)
			}
		}
	}
	return s, nil
}

func (s *linksScreen) View(w, h int) string {
	sty := s.d.sty
	if s.err != nil {
		return padLines(sty.Err.Render("links failed: ")+sty.Base.Render(s.err.Error()), w, h)
	}
	head := sty.Muted.Render("links from ") + sty.Title.Render(s.u.String())
	s.pick.setSize(w, max(1, h-2))
	return head + "\n\n" + s.pick.View(sty)
}

func (s *linksScreen) Title() string            { return "links " + s.u.Authority + "/" + s.u.ID() }
func (s *linksScreen) Subject() (kit.URI, bool) { return s.u, true }
func (s *linksScreen) Loading() bool            { return s.inflight }
func (s *linksScreen) ShortHelp() []key.Binding {
	k := s.d.keys
	return []key.Binding{k.Enter, k.Down, k.Back}
}
func (s *linksScreen) FullHelp() [][]key.Binding {
	k := s.d.keys
	return [][]key.Binding{{k.Up, k.Down, k.Top, k.Bottom, k.Enter, k.Back}}
}
