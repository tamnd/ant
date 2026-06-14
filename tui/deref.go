package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// Deref is the slice of *ant.Engine the TUI renders, the exact seam the web
// console depends on (web.Deref). Keeping it an interface means the screens are
// driven by their Update logic alone and the whole program is testable against a
// network-free fake (8000_ant_tui §6.4, §17).
type Deref interface {
	Domains() []ant.DomainInfo
	Domain(scheme string) (ant.DomainInfo, bool)
	Resolve(input, on string) (kit.URI, error)
	URL(u kit.URI) (string, error)
	Get(ctx context.Context, u kit.URI) (kit.Envelope, error)
	Dereference(ctx context.Context, u kit.URI, refresh bool) (ant.Fetched, error)
	Lookup(u kit.URI) (ant.Fetched, bool)
	Cached(u kit.URI) bool
	BodyOf(env kit.Envelope) (string, bool)
	List(ctx context.Context, u kit.URI, n int) ([]kit.Envelope, error)
	Searchable(scheme string) bool
	Search(ctx context.Context, scheme, query string, n int) ([]kit.Envelope, error)
	Links(ctx context.Context, u kit.URI) ([]kit.URI, error)
	Walk(ctx context.Context, u kit.URI, depth int) (*ant.Graph, error)
	Export(ctx context.Context, u kit.URI, follow int, md bool) (*ant.ExportReport, error)
	LL(prefix string) ([]string, error)
	Root() string
}

// --- fetch keys -------------------------------------------------------------
//
// A fetch key names a single in-flight read so a screen can match a result to
// the request it is waiting for, even after late prefetch results arrive on a
// background screen (8000_ant_tui §11). The shapes mirror the web console's
// fetchKey so the two surfaces reason about jobs the same way.

func getKey(u kit.URI, refresh bool) string {
	if refresh {
		return "get:" + u.String() + ":fresh"
	}
	return "get:" + u.String()
}

func listKey(u kit.URI, n int) string { return "ls:" + u.String() + ":" + itoa(n) }
func walkKey(u kit.URI, d int) string { return "graph:" + u.String() + ":" + itoa(d) }
func searchKey(scheme string, n int, q string) string {
	return "search:" + scheme + ":" + itoa(n) + ":" + q
}
func llKey(prefix string) string { return "ll:" + prefix }
func renderKey(u kit.URI) string { return "render:" + u.String() }

// --- command constructors ---------------------------------------------------
//
// Every constructor closes over the program context and the Deref, so the IO
// runs off the render loop and is cancelled when the program's signal context
// is (8000_ant_tui §11.4). Lookup is intentionally absent here: it is a cheap,
// network-free cache read screens call inline to paint instantly, before the
// matching background Dereference is even issued.

func derefCmd(ctx context.Context, e Deref, u kit.URI, refresh bool) tea.Cmd {
	key := getKey(u, refresh)
	return func() tea.Msg {
		f, err := e.Dereference(ctx, u, refresh)
		return fetchedMsg{Key: key, URI: u, Refresh: refresh, Fetched: f, Err: err}
	}
}

func listCmd(ctx context.Context, e Deref, u kit.URI, n int) tea.Cmd {
	key := listKey(u, n)
	return func() tea.Msg {
		envs, err := e.List(ctx, u, n)
		return listedMsg{Key: key, URI: u, N: n, Envs: envs, Err: err}
	}
}

func walkCmd(ctx context.Context, e Deref, u kit.URI, depth int) tea.Cmd {
	key := walkKey(u, depth)
	return func() tea.Msg {
		g, err := e.Walk(ctx, u, depth)
		return walkedMsg{Key: key, URI: u, Depth: depth, Graph: g, Err: err}
	}
}

func searchCmd(ctx context.Context, e Deref, scheme, query string, n int) tea.Cmd {
	key := searchKey(scheme, n, query)
	return func() tea.Msg {
		envs, err := e.Search(ctx, scheme, query, n)
		return searchedMsg{Key: key, Scheme: scheme, Query: query, N: n, Envs: envs, Err: err}
	}
}

func llCmd(e Deref, prefix string) tea.Cmd {
	key := llKey(prefix)
	return func() tea.Msg {
		uris, err := e.LL(prefix)
		return llMsg{Key: key, Prefix: prefix, URIs: uris, Err: err}
	}
}

func resolveCmd(e Deref, input, on string) tea.Cmd {
	return func() tea.Msg {
		u, err := e.Resolve(input, on)
		return resolvedMsg{Input: input, On: on, URI: u, Err: err}
	}
}

func exportCmd(ctx context.Context, e Deref, u kit.URI, follow int, md bool) tea.Cmd {
	return func() tea.Msg {
		rep, err := e.Export(ctx, u, follow, md)
		return exportedMsg{URI: u, Report: rep, Err: err}
	}
}
