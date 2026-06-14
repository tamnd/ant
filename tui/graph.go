package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// graphScreen is the x-ray: it walks the subgraph reachable from a record within
// a depth and shows it as an indented tree of followable nodes, with a toggle to
// the raw DOT the CLI emits. '+'/'-' change the depth and re-walk (8000_ant_tui §9).
type graphScreen struct {
	screenBase
	u        kit.URI
	depth    int
	g        *ant.Graph
	pick     picker
	dot      viewport.Model
	dotMode  bool
	inflight bool
	err      error
}

func newGraphScreen(d *deps, u kit.URI, depth int) *graphScreen {
	if depth < 1 {
		depth = 1
	}
	s := &graphScreen{screenBase: screenBase{d: d}, u: u, depth: depth, pick: newPicker(), dot: viewport.New()}
	s.pick.empty = "No nodes."
	return s
}

func (s *graphScreen) Init() tea.Cmd {
	s.inflight = true
	return walkCmd(s.d.ctx, s.d.e, s.u, s.depth)
}

func (s *graphScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = msg.Width, msg.Height
	case walkedMsg:
		if msg.Key != walkKey(s.u, s.depth) {
			return s, nil
		}
		s.inflight = false
		if msg.Err != nil {
			s.err = msg.Err
			return s, nil
		}
		s.g = msg.Graph
		s.buildItems()
		s.dot.SetContent(s.d.sty.Base.Render(s.g.Dot()))
	case tea.KeyPressMsg:
		return s.onKey(msg)
	}
	return s, nil
}

func (s *graphScreen) onKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	k := s.d.keys
	switch {
	case key.Matches(msg, k.Mode):
		s.dotMode = !s.dotMode
		return s, nil
	case msg.String() == "+", msg.String() == "=":
		s.depth++
		s.inflight = true
		return s, walkCmd(s.d.ctx, s.d.e, s.u, s.depth)
	case msg.String() == "-", msg.String() == "_":
		if s.depth > 1 {
			s.depth--
			s.inflight = true
			return s, walkCmd(s.d.ctx, s.d.e, s.u, s.depth)
		}
		return s, nil
	case key.Matches(msg, k.Enter):
		if !s.dotMode {
			if it, ok := s.pick.selected(); ok && it.hasURI {
				return s, navigate(it.uri)
			}
		}
		return s, nil
	}
	if s.dotMode {
		s.dotMotion(msg, k)
		return s, nil
	}
	s.pick.handle(msg, k)
	return s, nil
}

func (s *graphScreen) dotMotion(msg tea.KeyPressMsg, k keyMap) {
	switch {
	case key.Matches(msg, k.Up):
		s.dot.ScrollUp(1)
	case key.Matches(msg, k.Down):
		s.dot.ScrollDown(1)
	case key.Matches(msg, k.Top):
		s.dot.GotoTop()
	case key.Matches(msg, k.Bottom):
		s.dot.GotoBottom()
	case key.Matches(msg, k.HalfUp):
		s.dot.HalfPageUp()
	case key.Matches(msg, k.HalfDown):
		s.dot.HalfPageDown()
	}
}

func (s *graphScreen) buildItems() {
	if s.g == nil {
		return
	}
	depth := bfsDepth(s.u.String(), s.g)
	var items []pickItem
	for _, n := range s.g.Nodes {
		indent := strings.Repeat("  ", depth[n.URI])
		if u, err := kit.ParseURI(n.URI); err == nil {
			items = append(items, pickItem{
				title: indent + u.Authority + "/" + u.ID(), subtitle: n.Type,
				scheme: u.Scheme, uri: u, hasURI: true,
			})
			continue
		}
		items = append(items, pickItem{title: indent + n.URI, subtitle: n.Type, scheme: schemeOf(n.Type)})
	}
	s.pick.setItems(items)
}

// bfsDepth assigns each node its hop-distance from the root over the graph's
// edges, so the tree view can indent by depth. Unreached nodes stay at 0.
func bfsDepth(root string, g *ant.Graph) map[string]int {
	adj := map[string][]string{}
	for _, e := range g.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	depth := map[string]int{root: 0}
	queue := []string{root}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, to := range adj[cur] {
			if _, seen := depth[to]; !seen {
				depth[to] = depth[cur] + 1
				queue = append(queue, to)
			}
		}
	}
	return depth
}

func (s *graphScreen) View(w, h int) string {
	sty := s.d.sty
	if s.err != nil {
		return padLines(sty.Err.Render("walk failed: ")+sty.Base.Render(s.err.Error()), w, h)
	}
	view := "tree"
	if s.dotMode {
		view = "dot"
	}
	counts := ""
	if s.g != nil {
		counts = fmt.Sprintf(" · %d nodes · %d edges", len(s.g.Nodes), len(s.g.Edges))
	}
	head := sty.Muted.Render("graph of ") + sty.Title.Render(s.u.String()) +
		sty.Muted.Render(fmt.Sprintf("  depth %d · %s%s", s.depth, view, counts))
	body := max(1, h-2)
	if s.dotMode {
		s.dot.SetWidth(w)
		s.dot.SetHeight(body)
		return head + "\n\n" + s.dot.View()
	}
	s.pick.setSize(w, body)
	return head + "\n\n" + s.pick.View(sty)
}

func (s *graphScreen) Title() string            { return "graph " + s.u.Authority + "/" + s.u.ID() }
func (s *graphScreen) Subject() (kit.URI, bool) { return s.u, true }
func (s *graphScreen) Loading() bool            { return s.inflight }
func (s *graphScreen) ShortHelp() []key.Binding {
	k := s.d.keys
	return []key.Binding{k.Enter, k.Mode, k.Down, k.Back}
}
func (s *graphScreen) FullHelp() [][]key.Binding {
	k := s.d.keys
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom, k.Enter},
		{k.Mode, k.Back},
	}
}
