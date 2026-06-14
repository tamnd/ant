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

// contentMode is which face of a record the content pane shows.
type contentMode int

const (
	modeData contentMode = iota
	modeBody
	modeRaw
)

func modeName(m contentMode) string {
	switch m {
	case modeBody:
		return "body"
	case modeRaw:
		return "raw"
	default:
		return "data"
	}
}

// focusArea is which of the resource screen's two panes has the keyboard.
type focusArea int

const (
	focusContent focusArea = iota
	focusLinks
)

// resourceScreen is the heart of the program: one dereferenced record. It paints
// instantly from the cache (Lookup) and, on a miss or a refresh, fills in when
// the background Dereference returns. The content pane shows the record's data,
// its rendered body, or its raw JSON; the links pane lists the typed edges, and
// Enter on one follows it (8000_ant_tui §7.2, §8).
type resourceScreen struct {
	screenBase
	u       kit.URI
	refresh bool

	f         ant.Fetched
	loaded    bool
	fromCache bool
	loadErr   error
	inflight  bool

	mode    contentMode
	focus   focusArea
	content viewport.Model
	links   picker

	bodyRendered  string
	bodyRequested bool

	// content-pane render memo, so the data/raw block re-renders only on a real
	// change rather than every frame.
	lastW        int
	lastMode     contentMode
	contentDirty bool
}

func newResourceScreen(d *deps, u kit.URI, refresh bool) *resourceScreen {
	s := &resourceScreen{
		screenBase: screenBase{d: d},
		u:          u,
		refresh:    refresh,
		mode:       modeData,
		content:    viewport.New(),
		links:      newPicker(),
	}
	s.links.empty = "No outbound links."
	return s
}

func (s *resourceScreen) Init() tea.Cmd {
	if f, ok := s.d.e.Lookup(s.u); ok {
		s.install(f)
		if !s.refresh {
			return nil
		}
	}
	s.inflight = true
	return derefCmd(s.d.ctx, s.d.e, s.u, s.refresh)
}

func (s *resourceScreen) install(f ant.Fetched) {
	s.f = f
	s.loaded = true
	s.loadErr = nil
	s.fromCache = f.FromCache
	s.links.setItems(linkItemsFrom(f.Env.Links))
	s.contentDirty = true
}

func (s *resourceScreen) Update(msg tea.Msg) (Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.w, s.h = msg.Width, msg.Height
		s.contentDirty = true
		if s.mode == modeBody {
			s.bodyRendered, s.bodyRequested = "", false
			return s, s.maybeRenderBody()
		}
		return s, nil

	case fetchedMsg:
		if msg.URI.String() != s.u.String() {
			return s, nil
		}
		s.inflight = false
		if msg.Err != nil {
			if !s.loaded {
				s.loadErr = msg.Err
			}
			return s, nil
		}
		s.install(msg.Fetched)
		return s, s.maybeRenderBody()

	case renderedMsg:
		if msg.Key == renderKey(s.u) {
			s.bodyRendered = msg.Markdown
			if s.mode == modeBody {
				s.contentDirty = true
			}
		}
		return s, nil

	case tea.KeyPressMsg:
		return s.onKey(msg)
	}
	return s, nil
}

func (s *resourceScreen) onKey(msg tea.KeyPressMsg) (Screen, tea.Cmd) {
	k := s.d.keys
	switch {
	case key.Matches(msg, k.Tab):
		if len(s.links.items) > 0 {
			if s.focus == focusContent {
				s.focus = focusLinks
			} else {
				s.focus = focusContent
			}
		}
		return s, nil
	case key.Matches(msg, k.Mode):
		s.cycleMode()
		return s, s.maybeRenderBody()
	case key.Matches(msg, k.Body):
		s.mode = modeBody
		s.contentDirty = true
		return s, s.maybeRenderBody()
	case key.Matches(msg, k.Refresh):
		s.refresh, s.inflight = true, true
		return s, derefCmd(s.d.ctx, s.d.e, s.u, true)
	case key.Matches(msg, k.Links):
		return s, push(newLinksScreen(s.d, s.u))
	case key.Matches(msg, k.List):
		return s, push(newCollectionScreen(s.d, s.u))
	case key.Matches(msg, k.Graph):
		return s, push(newGraphScreen(s.d, s.u, 1))
	case key.Matches(msg, k.URL):
		loc, err := s.d.e.URL(s.u)
		if err != nil {
			return s, toastCmd("no live url: "+err.Error(), true)
		}
		return s, tea.Batch(tea.SetClipboard(loc), toastCmd("copied live url: "+loc, false))
	case key.Matches(msg, k.Copy):
		return s, tea.Batch(tea.SetClipboard(s.u.String()), toastCmd("copied "+s.u.String(), false))
	case key.Matches(msg, k.Export):
		return s, tea.Batch(toastCmd("exporting "+s.u.String()+" …", false), exportCmd(s.d.ctx, s.d.e, s.u, 0, false))
	case key.Matches(msg, k.Enter):
		if s.focus == focusLinks {
			if it, ok := s.links.selected(); ok && it.hasURI {
				return s, navigate(it.uri)
			}
		}
		return s, nil
	}
	if s.focus == focusLinks {
		s.links.handle(msg, k)
		return s, nil
	}
	s.contentMotion(msg, k)
	return s, nil
}

func (s *resourceScreen) contentMotion(msg tea.KeyPressMsg, k keyMap) {
	switch {
	case key.Matches(msg, k.Up):
		s.content.ScrollUp(1)
	case key.Matches(msg, k.Down):
		s.content.ScrollDown(1)
	case key.Matches(msg, k.Top):
		s.content.GotoTop()
	case key.Matches(msg, k.Bottom):
		s.content.GotoBottom()
	case key.Matches(msg, k.HalfUp):
		s.content.HalfPageUp()
	case key.Matches(msg, k.HalfDown):
		s.content.HalfPageDown()
	}
}

func (s *resourceScreen) cycleMode() {
	switch s.mode {
	case modeData:
		if s.f.HasBody {
			s.mode = modeBody
		} else {
			s.mode = modeRaw
		}
	case modeBody:
		s.mode = modeRaw
	default:
		s.mode = modeData
	}
	s.contentDirty = true
}

// maybeRenderBody kicks off the glamour pass when body mode needs it, off the
// render loop. The result returns as a renderedMsg keyed to this record.
func (s *resourceScreen) maybeRenderBody() tea.Cmd {
	if s.mode != modeBody || !s.loaded || !s.f.HasBody || s.bodyRendered != "" || s.bodyRequested {
		return nil
	}
	s.bodyRequested = true
	return renderBodyCmd(renderKey(s.u), s.f.Body, s.d.sty.dark, s.w)
}

func (s *resourceScreen) View(w, h int) string {
	s.w, s.h = w, h
	sty := s.d.sty
	if !s.loaded {
		if s.loadErr != nil {
			return s.errView(sty, w, h)
		}
		return sty.Muted.Render("Loading " + s.u.String() + " …")
	}

	contentH, linksH := s.layout()
	s.content.SetWidth(w)
	s.content.SetHeight(contentH)
	if s.contentDirty || s.lastW != w || s.lastMode != s.mode {
		s.content.SetContent(s.renderContent(w))
		s.lastW, s.lastMode, s.contentDirty = w, s.mode, false
	}

	parts := []string{
		s.header(sty, w),
		s.sectionTitle(sty, strings.ToUpper(modeName(s.mode)), s.focus == focusContent),
		s.content.View(),
	}
	if linksH > 0 {
		s.links.setSize(w, linksH)
		parts = append(parts,
			s.sectionTitle(sty, fmt.Sprintf("LINKS (%d)", len(s.links.items)), s.focus == focusLinks),
			s.links.View(sty))
	}
	return strings.Join(parts, "\n")
}

// layout splits the body height between the content pane and the links pane,
// reserving a line for each section title.
func (s *resourceScreen) layout() (contentH, linksH int) {
	remaining := s.h - 2 // header
	if remaining < 1 {
		remaining = 1
	}
	if len(s.links.items) > 0 {
		linksH = len(s.links.items)
		if cap := max(3, remaining/3); linksH > cap {
			linksH = cap
		}
		contentH = remaining - 2 - linksH
	} else {
		contentH = remaining - 1
	}
	if contentH < 1 {
		contentH = 1
	}
	return
}

func (s *resourceScreen) renderContent(width int) string {
	sty := s.d.sty
	switch s.mode {
	case modeRaw:
		return renderJSON(s.f.Raw, sty)
	case modeBody:
		if !s.f.HasBody {
			return sty.Muted.Render("(this record has no body)")
		}
		if s.bodyRendered == "" {
			return sty.Muted.Render("rendering …")
		}
		return s.bodyRendered
	default:
		return renderData(s.f.Raw, sty, width)
	}
}

func (s *resourceScreen) header(sty *styles, w int) string {
	scheme := schemeOf(s.f.Env.Type)
	line1 := sty.schemeStyle(scheme).Render("● "+s.f.Env.Type) + "  " + sty.Title.Render(s.u.String())
	badge := sty.Muted.Render("live")
	if s.fromCache {
		badge = sty.OK.Render("cached")
	}
	line2 := sty.Muted.Render(relTime(s.f.Env.Fetched)) + sty.Crumb.Render(" · ") + badge
	return clampLine(line1, w) + "\n" + clampLine(line2, w)
}

func (s *resourceScreen) sectionTitle(sty *styles, label string, focused bool) string {
	if focused {
		return sty.Title.Render("▌ " + label)
	}
	return sty.Muted.Render("  " + label)
}

func (s *resourceScreen) errView(sty *styles, w, h int) string {
	block := sty.Err.Render("Couldn't load "+s.u.String()) + "\n\n" +
		sty.Base.Render(s.loadErr.Error()) + "\n\n" +
		sty.Muted.Render("r retry · u live url · esc back")
	return padLines(block, w, h)
}

func (s *resourceScreen) Title() string {
	if s.u.ID() != "" {
		return s.u.Authority + "/" + s.u.ID()
	}
	return s.u.Authority
}

func (s *resourceScreen) Subject() (kit.URI, bool) { return s.u, true }
func (s *resourceScreen) Loading() bool            { return s.inflight }
func (s *resourceScreen) Cached() bool             { return s.loaded && s.fromCache && !s.inflight }

func (s *resourceScreen) ShortHelp() []key.Binding {
	k := s.d.keys
	return []key.Binding{k.Tab, k.Enter, k.Mode, k.Links, k.Refresh, k.Copy}
}

func (s *resourceScreen) FullHelp() [][]key.Binding {
	k := s.d.keys
	return [][]key.Binding{
		{k.Up, k.Down, k.HalfDown, k.HalfUp, k.Top, k.Bottom},
		{k.Tab, k.Mode, k.Body, k.Enter},
		{k.Links, k.List, k.Graph, k.URL, k.Copy, k.Export, k.Refresh},
	}
}

// linkItemsFrom turns an envelope's grouped @links into picker rows, labeled by
// target with the source field as the dim subtitle.
func linkItemsFrom(m map[string][]string) []pickItem {
	rows := flattenLinks(m)
	items := make([]pickItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, pickItem{
			title:    r.URI.Authority + "/" + r.URI.ID(),
			subtitle: humanize(r.Field),
			scheme:   r.URI.Scheme,
			uri:      r.URI,
			hasURI:   true,
		})
	}
	return items
}
