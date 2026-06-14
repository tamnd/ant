package tui

import (
	"context"
	"encoding/json"

	tea "charm.land/bubbletea/v2"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// fakeDeref is a network-free stand-in for *ant.Engine, so every screen can be
// driven and asserted in a unit test (8000_ant_tui §17). It mirrors the web
// console's fake so the two surfaces are exercised against the same shapes.
type fakeDeref struct {
	cold bool // when set, Lookup misses so the cold background-deref path is taken
}

func mustURI(s string) kit.URI {
	u, err := kit.ParseURI(s)
	if err != nil {
		panic(err)
	}
	return u
}

func (fakeDeref) env(u kit.URI) kit.Envelope {
	return kit.Envelope{
		ID:      u.String(),
		Type:    "demo/" + u.Authority,
		Fetched: "2026-06-14T08:00:00Z",
		Links:   map[string][]string{"maker_id": {"demo://maker/m1"}},
		Data: map[string]any{
			"id":          u.ID(),
			"name":        "Widget " + u.ID(),
			"description": "a demo record",
			"maker_id":    "demo://maker/m1",
		},
	}
}

func (fakeDeref) Domains() []ant.DomainInfo {
	return []ant.DomainInfo{{
		Scheme: "demo", Aliases: []string{"dm"}, Hosts: []string{"demo.example"},
		Binary: "demo", Short: "A demo domain", Site: "https://demo.example",
		Repo: "https://example.com/demo",
	}}
}

func (f fakeDeref) Domain(s string) (ant.DomainInfo, bool) {
	for _, d := range f.Domains() {
		if d.Scheme == s {
			return d, true
		}
	}
	return ant.DomainInfo{}, false
}

func (fakeDeref) Resolve(input, on string) (kit.URI, error) { return mustURI("demo://widget/42"), nil }
func (fakeDeref) URL(u kit.URI) (string, error)             { return "https://demo.example/" + u.ID(), nil }

func (f fakeDeref) Get(_ context.Context, u kit.URI) (kit.Envelope, error) { return f.env(u), nil }

func (f fakeDeref) Dereference(_ context.Context, u kit.URI, refresh bool) (ant.Fetched, error) {
	env := f.env(u)
	raw, _ := json.MarshalIndent(env, "", "  ")
	return ant.Fetched{Env: env, Raw: raw, Body: "# Body\n\nHello from the body.", HasBody: true, FromCache: !refresh}, nil
}

func (f fakeDeref) Lookup(u kit.URI) (ant.Fetched, bool) {
	if f.cold {
		return ant.Fetched{}, false
	}
	env := f.env(u)
	raw, _ := json.MarshalIndent(env, "", "  ")
	return ant.Fetched{Env: env, Raw: raw, Body: "# Body\n\nHello from the body.", HasBody: true, FromCache: true}, true
}

func (f fakeDeref) Cached(u kit.URI) bool            { return !f.cold }
func (fakeDeref) BodyOf(kit.Envelope) (string, bool) { return "body", true }

func (f fakeDeref) List(_ context.Context, u kit.URI, n int) ([]kit.Envelope, error) {
	return []kit.Envelope{f.env(mustURI("demo://widget/1")), f.env(mustURI("demo://widget/2"))}, nil
}

func (fakeDeref) Searchable(s string) bool { return s == "demo" }

func (f fakeDeref) Search(_ context.Context, scheme, q string, n int) ([]kit.Envelope, error) {
	return []kit.Envelope{f.env(mustURI("demo://widget/7"))}, nil
}

func (fakeDeref) Links(_ context.Context, u kit.URI) ([]kit.URI, error) {
	return []kit.URI{mustURI("demo://maker/m1")}, nil
}

func (fakeDeref) Walk(_ context.Context, u kit.URI, depth int) (*ant.Graph, error) {
	return &ant.Graph{
		Nodes: []ant.GraphNode{
			{URI: "demo://widget/42", Type: "demo/widget"},
			{URI: "demo://maker/m1", Type: "demo/maker"},
		},
		Edges: []ant.GraphEdge{{From: "demo://widget/42", To: "demo://maker/m1"}},
	}, nil
}

func (fakeDeref) Export(_ context.Context, u kit.URI, follow int, md bool) (*ant.ExportReport, error) {
	return &ant.ExportReport{Root: u.String(), Written: []string{u.String()}}, nil
}

func (fakeDeref) LL(prefix string) ([]string, error) {
	return []string{"demo://widget/42", "demo://widget/1", "demo://maker/m1"}, nil
}

func (fakeDeref) Root() string { return "/tmp/data" }

// --- test helpers -----------------------------------------------------------

func testDeps(e Deref) *deps {
	return &deps{
		e:     e,
		ctx:   context.Background(),
		keys:  newKeys(),
		sty:   newStyles(true),
		build: Build{Version: "test", Commit: "abc1234", Date: "2026-06-14"},
	}
}

// drain runs a command to its message, flattening a Batch into the list of every
// message its child commands produce, so a test can find the one it cares about.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, drain(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// key constructors for tests.
func kRune(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }
func kCode(c rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: c} }

var (
	kEnter = kCode(tea.KeyEnter)
	kEsc   = kCode(tea.KeyEscape)
	kDown  = kCode(tea.KeyDown)
	kTab   = kCode(tea.KeyTab)
)

func sized(s Screen, w, h int) Screen {
	s, _ = s.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return s
}
