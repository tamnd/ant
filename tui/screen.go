package tui

import (
	"context"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/tamnd/any-cli/kit"
)

// deps is the shared environment every screen is born with: the Deref it reads
// through, the program context that cancels its IO, the keymap, and a pointer to
// the live styles. The App owns one deps and hands it to each screen
// constructor; a theme toggle mutates deps.sty in place, so every screen, even
// one resting in the back-stack, repaints in the new palette (8000_ant_tui §13).
type deps struct {
	e     Deref
	ctx   context.Context
	keys  keyMap
	sty   *styles
	build Build
}

// Screen is one place in the program: a render plus the Update that drives it.
// It is the unit the App pushes and pops. A screen owns its own scroll position
// and loaded data, so going back restores it exactly as left, the place k9s
// loses (8000_ant_tui §5.2, §12).
type Screen interface {
	// Init returns the command that kicks off the screen's first load. The App
	// runs it after sizing the screen.
	Init() tea.Cmd
	// Update advances the screen and returns its next self plus any command. A
	// screen never mutates the stack directly: it asks via navigate/push/back
	// messages carried on the returned command.
	Update(tea.Msg) (Screen, tea.Cmd)
	// View renders the screen body into the given content box (the area between
	// the title and status bars).
	View(width, height int) string
	// Title is the short label shown in the title bar and breadcrumb.
	Title() string
	// Subject is the URI this screen is about, if any, for the omnibox default
	// and the copy/url/graph actions.
	Subject() (kit.URI, bool)
	// Loading reports whether a fetch this screen issued is still in flight, so
	// the App shows the spinner.
	Loading() bool
	// Cached reports whether the screen is showing a record served from the
	// on-disk cache, so the title bar can badge it (8000_ant_tui §4).
	Cached() bool
	// Capturing reports that the screen wants raw key input (a text field is
	// focused), so the App routes every key to it and suspends the global keymap,
	// the same deal the omnibox gets while open (8000_ant_tui §10.3).
	Capturing() bool
	// ShortHelp is the footer key hints; FullHelp the help overlay grid.
	ShortHelp() []key.Binding
	FullHelp() [][]key.Binding
}

// screenBase carries the fields and the default method bodies every screen
// shares, so a screen file holds only what makes it distinct. A screen embeds it
// and overrides the methods it actually implements (Loading, Cached, Subject,
// Capturing) as needed.
type screenBase struct {
	d    *deps
	w, h int
}

func (s *screenBase) Capturing() bool          { return false }
func (s *screenBase) Cached() bool             { return false }
func (s *screenBase) Loading() bool            { return false }
func (s *screenBase) Subject() (kit.URI, bool) { return kit.URI{}, false }

// --- navigation command builders --------------------------------------------
//
// Screens request a stack change by returning one of these commands; the App is
// the only code that mutates the stack (8000_ant_tui §5.1).

func navigate(u kit.URI) tea.Cmd { return func() tea.Msg { return navigateMsg{URI: u} } }
func navigateFresh(u kit.URI) tea.Cmd {
	return func() tea.Msg { return navigateMsg{URI: u, Refresh: true} }
}
func push(s Screen) tea.Cmd    { return func() tea.Msg { return pushMsg{Screen: s} } }
func replace(s Screen) tea.Cmd { return func() tea.Msg { return replaceMsg{Screen: s} } }
func goBack() tea.Cmd          { return func() tea.Msg { return backMsg{} } }

// itoa is a tiny strconv.Itoa alias used by the fetch-key builders.
func itoa(n int) string { return strconv.Itoa(n) }

// schemeOf pulls the leading scheme out of a record type ("demo/widget" ->
// "demo") for the accent and badge color.
func schemeOf(typ string) string {
	if i := strings.IndexByte(typ, '/'); i >= 0 {
		return typ[:i]
	}
	return typ
}
