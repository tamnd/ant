package tui

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/tamnd/any-cli/kit"
)

// browseScreen turns the on-disk data tree into a navigable column. The root
// lists the registered domains as folders (scoped to ant's own data, never the
// shared $HOME/data root); a scheme prefix lists the records cached under it,
// grouping any deeper segments back into folders. Descending pushes a new browse
// screen so the back-stack is the path you walked, the same level-navigation the
// web console's breadcrumb gives. It is pure filesystem work, always offline-safe
// (8000_ant_tui §7.8).
type browseScreen struct {
	screenBase
	prefix   string   // canonical, e.g. "" or "goodreads://" or "goodreads://book"
	segs     []string // splitPrefix(prefix)
	all      []pickItem
	pick     picker
	ti       textinput.Model
	editing  bool
	counts   map[string]int // root only: scheme -> cached record count
	inflight int
	err      error
}

func newBrowseScreen(d *deps, prefix string) *browseScreen {
	segs := splitPrefix(prefix)
	ti := textinput.New()
	ti.Prompt = ""
	ti.SetVirtualCursor(true)
	ti.Placeholder = "filter"
	s := &browseScreen{
		screenBase: screenBase{d: d},
		prefix:     joinPrefix(segs),
		segs:       segs,
		pick:       newPicker(),
		ti:         ti,
		counts:     map[string]int{},
	}
	s.pick.empty = "Nothing cached here yet."
	return s
}

func (s *browseScreen) Init() tea.Cmd {
	if len(s.segs) == 0 {
		s.buildRoot()
		var cmds []tea.Cmd
		for _, dom := range s.d.e.Domains() {
			s.inflight++
			cmds = append(cmds, llCmd(s.d.e, dom.Scheme+"://"))
		}
		return tea.Batch(cmds...)
	}
	s.inflight = 1
	return llCmd(s.d.e, s.prefix)
}

func (s *browseScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = msg.Width, msg.Height
		if s.w > 4 {
			s.ti.SetWidth(s.w - 4)
		}
	case llMsg:
		return s.onLL(msg)
	case tea.KeyPressMsg:
		return s.onKey(msg)
	}
	return s, nil
}

func (s *browseScreen) onLL(msg llMsg) (Screen, tea.Cmd) {
	if len(s.segs) == 0 {
		// One probe per domain at the root; match by prefix and refresh the count.
		sc := strings.TrimSuffix(msg.Prefix, "://")
		if msg.Err == nil {
			s.counts[sc] = len(msg.URIs)
		}
		if s.inflight > 0 {
			s.inflight--
		}
		s.buildRoot()
		s.applyFilter()
		return s, nil
	}
	if msg.Key != llKey(s.prefix) {
		return s, nil
	}
	s.inflight = 0
	if msg.Err != nil {
		s.err = msg.Err
		return s, nil
	}
	s.buildChildren(msg.URIs)
	s.applyFilter()
	return s, nil
}

func (s *browseScreen) onKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	k := s.d.keys
	if s.editing {
		switch {
		case key.Matches(msg, k.Escape):
			s.editing = false
			s.ti.Blur()
			return s, nil
		case key.Matches(msg, k.Enter):
			s.editing = false
			s.ti.Blur()
			return s, nil
		default:
			var cmd tea.Cmd
			s.ti, cmd = s.ti.Update(msg)
			s.applyFilter()
			return s, cmd
		}
	}
	if s.pick.handle(msg, k) {
		return s, nil
	}
	switch {
	case key.Matches(msg, k.Filter):
		s.editing = true
		return s, s.ti.Focus()
	case key.Matches(msg, k.Enter), key.Matches(msg, k.Right):
		if it, ok := s.pick.selected(); ok {
			if it.hasURI {
				return s, navigate(it.uri)
			}
			if it.payload != "" {
				return s, push(newBrowseScreen(s.d, it.payload))
			}
		}
	case key.Matches(msg, k.Left):
		// Ascend a level (a no-op at the root; the App keeps the dashboard below).
		if len(s.segs) > 0 {
			return s, goBack()
		}
	}
	return s, nil
}

// buildRoot lists the registered domains as folders, each carrying its cached
// record count once the per-domain probe lands.
func (s *browseScreen) buildRoot() {
	var items []pickItem
	for _, dom := range s.d.e.Domains() {
		sub := "domain"
		if c, ok := s.counts[dom.Scheme]; ok {
			sub = itoa(c) + " cached"
		}
		items = append(items, pickItem{
			title: dom.Scheme, subtitle: sub, scheme: dom.Scheme, payload: dom.Scheme + "://",
		})
	}
	s.all = items
}

// buildChildren groups the cached URIs under this prefix: a child with more
// segments below it is a folder, one that terminates here is a record.
func (s *browseScreen) buildChildren(uris []string) {
	depth := len(s.segs)
	folderCount := map[string]int{}
	var folderOrder []string
	var leaves []string
	for _, uri := range uris {
		us := uriSegs(uri)
		if len(us) <= depth || !segsHasPrefix(us, s.segs) {
			continue
		}
		child := us[depth]
		if len(us) == depth+1 {
			leaves = append(leaves, uri)
			continue
		}
		if _, seen := folderCount[child]; !seen {
			folderOrder = append(folderOrder, child)
		}
		folderCount[child]++
	}
	sort.Strings(folderOrder)
	sort.Strings(leaves)

	var items []pickItem
	for _, name := range folderOrder {
		child := joinPrefix(append(append([]string{}, s.segs...), name))
		items = append(items, pickItem{
			title: name + "/", subtitle: itoa(folderCount[name]) + " records",
			scheme: s.segs[0], payload: child,
		})
	}
	for _, uri := range leaves {
		if u, err := kit.ParseURI(uri); err == nil {
			items = append(items, pickItem{
				title: u.Authority + "/" + u.ID(), subtitle: "record",
				scheme: u.Scheme, uri: u, hasURI: true,
			})
		}
	}
	s.all = items
}

// applyFilter narrows the column to rows whose title contains the filter text.
func (s *browseScreen) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(s.ti.Value()))
	if q == "" {
		s.pick.setItems(s.all)
		return
	}
	var out []pickItem
	for _, it := range s.all {
		if strings.Contains(strings.ToLower(it.title), q) {
			out = append(out, it)
		}
	}
	s.pick.setItems(out)
}

func (s *browseScreen) View(w, h int) string {
	sty := s.d.sty
	if s.err != nil {
		return padLines(sty.Err.Render("browse failed: ")+sty.Base.Render(s.err.Error()), w, h)
	}
	loc := "data"
	if len(s.segs) > 0 {
		loc = "data / " + strings.Join(s.segs, " / ")
	}
	head := sty.Muted.Render("browse ") + sty.Title.Render(loc)
	rows := max(1, h-2)
	if s.editing {
		head += "\n" + sty.Crumb.Render("filter › ") + s.ti.View()
		rows = max(1, h-3)
	}
	s.pick.setSize(w, rows)
	return head + "\n\n" + s.pick.View(sty)
}

func (s *browseScreen) Title() string {
	if s.prefix == "" {
		return "browse"
	}
	return "browse " + s.prefix
}
func (s *browseScreen) Capturing() bool { return s.editing }
func (s *browseScreen) Loading() bool   { return s.inflight > 0 }
func (s *browseScreen) ShortHelp() []key.Binding {
	k := s.d.keys
	return []key.Binding{k.Enter, k.Left, k.Down, k.Filter, k.Back}
}
func (s *browseScreen) FullHelp() [][]key.Binding {
	k := s.d.keys
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Enter, k.Left, k.Right, k.Filter, k.Back},
	}
}

// --- data-tree prefix helpers (ported from web/render.go) --------------------

// splitPrefix turns a browse prefix ("", "goodreads://", "goodreads://book")
// into its data-tree segments ([], [goodreads], [goodreads book]).
func splitPrefix(prefix string) []string {
	p := strings.TrimSpace(prefix)
	if p == "" {
		return nil
	}
	p = strings.Replace(p, "://", "/", 1)
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// joinPrefix renders segments back into a prefix: a lone scheme becomes
// "scheme://", deeper segments append under it.
func joinPrefix(segs []string) string {
	switch len(segs) {
	case 0:
		return ""
	case 1:
		return segs[0] + "://"
	default:
		return segs[0] + "://" + strings.Join(segs[1:], "/")
	}
}

// uriSegs splits a record URI into [scheme, authority, id...], the same shape
// splitPrefix yields, so the two can be compared.
func uriSegs(uri string) []string {
	u, err := kit.ParseURI(uri)
	if err != nil {
		return nil
	}
	return append([]string{u.Scheme, u.Authority}, u.Path...)
}

// segsHasPrefix reports whether segs begins with pre.
func segsHasPrefix(segs, pre []string) bool {
	if len(segs) < len(pre) {
		return false
	}
	for i, p := range pre {
		if segs[i] != p {
			return false
		}
	}
	return true
}
