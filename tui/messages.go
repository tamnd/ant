package tui

import (
	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// The message vocabulary the program speaks (8000_ant_tui §5). Navigation
// requests (navigate/push/back/forward/home) flow up to the App, which owns the
// screen stack; data results (fetched/listed/walked/searched/ll/resolved/
// rendered) are broadcast to every screen so whichever is waiting on the key
// picks it up, including a background screen warmed by prefetch (§12).

// navigateMsg asks the App to open a Resource screen for a URI. It is the common
// "follow this link" request, emitted by the data pane, the links list, search
// results, the omnibox, and collection rows.
type navigateMsg struct {
	URI     kit.URI
	Refresh bool
}

// pushMsg asks the App to push an already-built screen (the typed screens a
// screen constructs itself: domain, search, graph, browse). The App sizes it and
// runs its Init, so a screen never has to know the chrome geometry.
type pushMsg struct{ Screen Screen }

// replaceMsg swaps the top screen in place rather than growing the stack, for an
// in-place refresh that should not add a back step.
type replaceMsg struct{ Screen Screen }

type backMsg struct{}
type forwardMsg struct{}
type homeMsg struct{}

// fetchedMsg carries a Dereference result back to the screen that requested Key.
type fetchedMsg struct {
	Key     string
	URI     kit.URI
	Refresh bool
	Fetched ant.Fetched
	Err     error
}

type listedMsg struct {
	Key  string
	URI  kit.URI
	N    int
	Envs []kit.Envelope
	Err  error
}

type walkedMsg struct {
	Key   string
	URI   kit.URI
	Depth int
	Graph *ant.Graph
	Err   error
}

type searchedMsg struct {
	Key    string
	Scheme string
	Query  string
	N      int
	Envs   []kit.Envelope
	Err    error
}

type llMsg struct {
	Key    string
	Prefix string
	URIs   []string
	Err    error
}

type resolvedMsg struct {
	Input string
	On    string
	URI   kit.URI
	Err   error
}

// renderedMsg carries a glamour-rendered body back to the screen, so the
// expensive Markdown pass runs off the render loop and its result is cached.
type renderedMsg struct {
	Key      string
	Markdown string
}

type exportedMsg struct {
	URI    kit.URI
	Report *ant.ExportReport
	Err    error
}

// toastMsg flashes a transient line in the status bar (a copied URL, a recovered
// error); errs render in the error color. clearToastMsg retires it.
type toastMsg struct {
	Text  string
	IsErr bool
}

type clearToastMsg struct{}
