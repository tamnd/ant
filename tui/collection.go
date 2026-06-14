package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/tamnd/any-cli/kit"
)

// collectionListN is how many members a collection screen asks for; the same
// default the CLI's ls uses.
const collectionListN = 40

// collectionScreen lists the members a URI expands to (ant ls): an author's
// books, a channel's videos, a category's pages. Each member is a followable row
// (8000_ant_tui §7.3).
type collectionScreen struct {
	screenBase
	u        kit.URI
	pick     picker
	inflight bool
	err      error
}

func newCollectionScreen(d *deps, u kit.URI) *collectionScreen {
	s := &collectionScreen{screenBase: screenBase{d: d}, u: u, pick: newPicker()}
	s.pick.empty = "No members."
	return s
}

func (s *collectionScreen) Init() tea.Cmd {
	s.inflight = true
	return listCmd(s.d.ctx, s.d.e, s.u, collectionListN)
}

func (s *collectionScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = msg.Width, msg.Height
	case listedMsg:
		if msg.Key != listKey(s.u, collectionListN) {
			return s, nil
		}
		s.inflight = false
		if msg.Err != nil {
			s.err = msg.Err
			return s, nil
		}
		s.pick.setItems(envItems(msg.Envs))
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

func (s *collectionScreen) View(w, h int) string {
	sty := s.d.sty
	if s.err != nil {
		return padLines(sty.Err.Render("ls failed: ")+sty.Base.Render(s.err.Error()), w, h)
	}
	head := sty.Muted.Render("members of ") + sty.Title.Render(s.u.String())
	s.pick.setSize(w, max(1, h-2))
	return head + "\n\n" + s.pick.View(sty)
}

func (s *collectionScreen) Title() string            { return "ls " + s.u.Authority + "/" + s.u.ID() }
func (s *collectionScreen) Subject() (kit.URI, bool) { return s.u, true }
func (s *collectionScreen) Loading() bool            { return s.inflight }
func (s *collectionScreen) ShortHelp() []key.Binding {
	k := s.d.keys
	return []key.Binding{k.Enter, k.Down, k.Back}
}
func (s *collectionScreen) FullHelp() [][]key.Binding {
	k := s.d.keys
	return [][]key.Binding{{k.Up, k.Down, k.Top, k.Bottom, k.Enter, k.Back}}
}
