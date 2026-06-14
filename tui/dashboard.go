package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// dashboardScreen is the landing screen and the domain index: it lists every
// registered driver as a followable row and points at the omnibox for jumping
// straight to a record (8000_ant_tui §7.1). It is always stack[0], so Home and a
// full back-stack unwind land here.
type dashboardScreen struct {
	screenBase
	pick picker
}

func newDashboardScreen(d *deps) *dashboardScreen {
	s := &dashboardScreen{screenBase: screenBase{d: d}, pick: newPicker()}
	s.pick.empty = "No domains registered."
	s.build()
	return s
}

func (s *dashboardScreen) build() {
	var items []pickItem
	for _, dom := range s.d.e.Domains() {
		title := dom.Scheme
		if len(dom.Aliases) > 0 {
			title += "  " + "(" + strings.Join(dom.Aliases, ", ") + ")"
		}
		items = append(items, pickItem{
			title:    title,
			subtitle: dom.Short,
			scheme:   dom.Scheme,
			payload:  dom.Scheme,
		})
	}
	s.pick.setItems(items)
}

func (s *dashboardScreen) Init() tea.Cmd { return nil }

func (s *dashboardScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if s.pick.handle(msg, s.d.keys) {
			return s, nil
		}
		switch {
		case key.Matches(msg, s.d.keys.Enter):
			if it, ok := s.pick.selected(); ok {
				return s, push(newDomainScreen(s.d, it.payload))
			}
		case key.Matches(msg, s.d.keys.Browse):
			return s, push(newBrowseScreen(s.d, ""))
		}
	}
	return s, nil
}

func (s *dashboardScreen) View(w, h int) string {
	sty := s.d.sty
	header := sty.Title.Render("Every record is a URI") + "\n" +
		sty.Muted.Render("Open a domain, or press : to go to any record.")
	s.pick.setSize(w, max(1, h-3))
	return header + "\n\n" + s.pick.View(sty)
}

func (s *dashboardScreen) Title() string { return "dashboard" }

func (s *dashboardScreen) ShortHelp() []key.Binding {
	k := s.d.keys
	return []key.Binding{k.Enter, k.Down, k.Browse, k.Omni}
}

func (s *dashboardScreen) FullHelp() [][]key.Binding {
	k := s.d.keys
	return [][]key.Binding{{k.Up, k.Down, k.Top, k.Bottom, k.Enter, k.Browse, k.Omni}}
}
