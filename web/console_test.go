package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// fakeDeref is a network-free stand-in for *ant.Engine, so the console's every
// page can be rendered and asserted in a unit test (8000_ant_serve §17).
type fakeDeref struct{}

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
func (f fakeDeref) Get(_ context.Context, u kit.URI) (kit.Envelope, error) {
	return f.env(u), nil
}
func (f fakeDeref) Dereference(_ context.Context, u kit.URI, refresh bool) (ant.Fetched, error) {
	env := f.env(u)
	raw, _ := json.MarshalIndent(env, "", "  ")
	return ant.Fetched{Env: env, Raw: raw, Body: "# Body\n\nHello from the body.", HasBody: true, FromCache: !refresh}, nil
}
func (fakeDeref) Cached(kit.URI) bool                { return true }
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
		Nodes: []ant.GraphNode{{URI: "demo://widget/42", Type: "demo/widget"}, {URI: "demo://maker/m1", Type: "demo/maker"}},
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

func newTestConsole(t *testing.T) *Console {
	t.Helper()
	c, err := New(fakeDeref{}, Build{Version: "test", Commit: "abc1234", Date: "2026-06-14"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestEveryPageRenders walks every HTML route and asserts each renders the shell
// with a 200 and the page-specific marker, so a template break is caught here
// rather than in the browser.
func TestEveryPageRenders(t *testing.T) {
	h := newTestConsole(t).Handler()

	cases := []struct {
		name, path, want string
		status           int
	}{
		{"dashboard", "/", "Every record is a URI", 200},
		{"resource", "/view?uri=demo://widget/42", "Fields", 200},
		{"resource-raw", "/demo://widget/42", "Fields", 200},
		{"collection", "/ls?uri=demo://widget/42", "Members", 200},
		{"search-empty", "/search", "Run a domain", 200},
		{"search-results", "/search?on=demo&q=gears", "result", 200},
		{"links", "/links?uri=demo://widget/42", "Links", 200},
		{"resolve", "/resolve", "Resolve", 200},
		{"locate", "/url?uri=demo://widget/42", "Live URL", 200},
		{"graph", "/graph?uri=demo://widget/42", "Graph", 200},
		{"browse-root", "/browse", "Data tree", 200},
		{"browse-scheme", "/browse?prefix=demo://", "demo", 200},
		{"browse-authority", "/browse?prefix=demo://widget", "Records", 200},
		{"domain", "/domain?scheme=demo", "demo", 200},
		{"about", "/about", "About ant", 200},
		{"error", "/view?uri=not%20a%20uri", "valid URI", 400},
		{"notfound", "/no-such-page", "Nothing here", 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			req.Header.Set("Accept", "text/html")
			h.ServeHTTP(rec, req)

			if rec.Code != c.status {
				t.Fatalf("%s: code %d, want %d\n%s", c.path, rec.Code, c.status, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, "<!doctype html>") {
				t.Errorf("%s: missing HTML shell", c.path)
			}
			if !strings.Contains(body, c.want) {
				t.Errorf("%s: body does not contain %q", c.path, c.want)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("%s: content-type %q", c.path, ct)
			}
			if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "nonce-") {
				t.Errorf("%s: missing CSP nonce", c.path)
			}
		})
	}
}

// TestJSONNegotiation asserts the same routes answer JSON without an HTML Accept,
// and that the /api prefix forces it.
func TestJSONNegotiation(t *testing.T) {
	h := newTestConsole(t).Handler()
	cases := []struct{ path, want string }{
		{"/resolve?input=x", `"uri"`},
		{"/url?uri=demo://widget/42", `"url"`},
		{"/view?uri=demo://widget/42", `"@id"`},
		{"/api/about", `"Version"`},
		{"/search?on=demo&q=gears", `"@id"`},
		{"/browse?prefix=demo://", `demo://widget/42`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: code %d", c.path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("%s: content-type %q, want json", c.path, ct)
		}
		if !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: body %q lacks %q", c.path, rec.Body.String(), c.want)
		}
	}
}

// recordingDeref records the prefixes passed to LL, so a test can assert which
// listings a page asks for.
type recordingDeref struct {
	fakeDeref
	mu      sync.Mutex
	llCalls []string
}

func (d *recordingDeref) LL(prefix string) ([]string, error) {
	d.mu.Lock()
	d.llCalls = append(d.llCalls, prefix)
	d.mu.Unlock()
	return d.fakeDeref.LL(prefix)
}

func (d *recordingDeref) calls() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.llCalls...)
}

// TestNoWholeTreeWalk guards the performance regression that made the dashboard
// and browse-root take seconds: they must never list the whole shared data root
// (LL with an empty prefix), only ant's own per-domain subtrees.
func TestNoWholeTreeWalk(t *testing.T) {
	for _, path := range []string{"/", "/browse"} {
		t.Run(path, func(t *testing.T) {
			rec := &recordingDeref{}
			c, err := New(rec, Build{Version: "test"})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Accept", "text/html")
			c.Handler().ServeHTTP(httptest.NewRecorder(), req)

			calls := rec.calls()
			sawDomain := false
			for _, p := range calls {
				if p == "" {
					t.Errorf("%s walked the whole data root (LL(%q)); calls=%v", path, p, calls)
				}
				if p == "demo://" {
					sawDomain = true
				}
			}
			if !sawDomain {
				t.Errorf("%s never listed the demo domain; calls=%v", path, calls)
			}
		})
	}
}

// TestAboutVersion is a quick assertion that build info reaches the page.
func TestAboutVersion(t *testing.T) {
	h := newTestConsole(t).Handler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/about", nil)
	req.Header.Set("Accept", "text/html")
	h.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "abc1234") {
		t.Error("about page missing commit")
	}
}
