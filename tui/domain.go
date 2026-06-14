package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// exampleURIs offers a couple of try-me URIs per known scheme, mirroring the web
// console's domain page so a reader has somewhere to start. Unknown drivers get
// none, which is better than a fabricated id (8000_ant_tui §7.9).
var exampleURIs = map[string][]string{
	"goodreads": {"goodreads://book/2767052", "goodreads://author/153394"},
	"x":         {"x://user/nasa", "x://status/20"},
	"wikipedia": {"wikipedia://page/Alan_Turing", "wikipedia://category/Computability_theory"},
	"youtube":   {"youtube://video/dQw4w9WgXcQ", "youtube://channel/UCuAXFkgsw1L7xaCfnd5JJOw"},
}

// domainScreen is one driver's home: its identity (aliases, hosts, site, repo),
// a few example records to open, and the records already cached under it, with a
// jump into that domain's search.
type domainScreen struct {
	scheme     string
	info       ant.DomainInfo
	hasInfo    bool
	searchable bool
	examples   []pickItem
	recent     []pickItem
	pick       picker
	inflight   bool
	screenBase
}

func newDomainScreen(d *deps, scheme string) *domainScreen {
	s := &domainScreen{screenBase: screenBase{d: d}, scheme: scheme, pick: newPicker()}
	s.info, s.hasInfo = d.e.Domain(scheme)
	s.searchable = d.e.Searchable(scheme)
	for _, ex := range exampleURIs[scheme] {
		if u, err := kit.ParseURI(ex); err == nil {
			s.examples = append(s.examples, pickItem{
				title: u.Authority + "/" + u.ID(), subtitle: "example", scheme: scheme, uri: u, hasURI: true,
			})
		}
	}
	s.rebuild()
	return s
}

func (s *domainScreen) rebuild() {
	items := append([]pickItem(nil), s.examples...)
	items = append(items, s.recent...)
	if len(items) == 0 {
		s.pick.empty = "Nothing cached yet. Press : to open a record."
	}
	s.pick.setItems(items)
}

func (s *domainScreen) Init() tea.Cmd {
	s.inflight = true
	return llCmd(s.d.e, s.scheme+"://")
}

func (s *domainScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = msg.Width, msg.Height
	case llMsg:
		if msg.Key != llKey(s.scheme+"://") {
			return s, nil
		}
		s.inflight = false
		if msg.Err == nil {
			var recent []pickItem
			for _, raw := range msg.URIs {
				if u, err := kit.ParseURI(raw); err == nil {
					recent = append(recent, pickItem{
						title: u.Authority + "/" + u.ID(), subtitle: "cached", scheme: s.scheme, uri: u, hasURI: true,
					})
				}
			}
			s.recent = recent
			s.rebuild()
		}
	case tea.KeyPressMsg:
		if s.pick.handle(msg, s.d.keys) {
			return s, nil
		}
		switch {
		case key.Matches(msg, s.d.keys.Enter):
			if it, ok := s.pick.selected(); ok && it.hasURI {
				return s, navigate(it.uri)
			}
		case key.Matches(msg, s.d.keys.Filter), key.Matches(msg, s.d.keys.List):
			if s.searchable {
				return s, push(newSearchScreen(s.d, s.scheme))
			}
		case key.Matches(msg, s.d.keys.Browse):
			return s, push(newBrowseScreen(s.d, s.scheme+"://"))
		}
	}
	return s, nil
}

func (s *domainScreen) View(w, h int) string {
	sty := s.d.sty
	var head strings.Builder
	head.WriteString(sty.schemeStyle(s.scheme).Render("● "+s.scheme) + "  ")
	if s.hasInfo && len(s.info.Aliases) > 0 {
		head.WriteString(sty.Muted.Render("(" + strings.Join(s.info.Aliases, ", ") + ")"))
	}
	head.WriteByte('\n')
	if s.hasInfo {
		if s.info.Short != "" {
			head.WriteString(sty.Base.Render(s.info.Short) + "\n")
		}
		var meta []string
		if s.info.Site != "" {
			meta = append(meta, "site "+s.info.Site)
		}
		if len(s.info.Hosts) > 0 {
			meta = append(meta, "hosts "+strings.Join(s.info.Hosts, ", "))
		}
		if s.searchable {
			meta = append(meta, "searchable (/)")
		}
		if len(meta) > 0 {
			head.WriteString(sty.Muted.Render(strings.Join(meta, sty.Crumb.Render(" · "))) + "\n")
		}
	}
	header := head.String()
	lines := strings.Count(header, "\n")
	s.pick.setSize(w, max(1, h-lines-1))
	return header + "\n" + s.pick.View(sty)
}

func (s *domainScreen) Title() string { return "domain " + s.scheme }
func (s *domainScreen) Loading() bool { return s.inflight }
func (s *domainScreen) ShortHelp() []key.Binding {
	k := s.d.keys
	out := []key.Binding{k.Enter, k.Down}
	if s.searchable {
		out = append(out, k.Filter)
	}
	return append(out, k.Browse, k.Back)
}
func (s *domainScreen) FullHelp() [][]key.Binding {
	k := s.d.keys
	return [][]key.Binding{{k.Up, k.Down, k.Top, k.Bottom, k.Enter, k.Filter, k.Back}}
}
