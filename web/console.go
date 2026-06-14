// Package web is the ant web console: a browser GUI over the whole resource-URI
// namespace, server-rendered in pure Go and styled to match shadcn/ui, with the
// machine-facing JSON API preserved under content negotiation. It is the human
// surface that sits beside the CLI and the MCP server (8000_ant_serve).
//
// The console adds no data capability of its own; every page is a thin rendering
// of an ant.Engine method. It depends only on the Deref interface, so it is
// testable against a fake and could later be mounted by another host.
package web

import (
	"context"
	"html/template"
	"io/fs"
	"net/http"
	"strings"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// Deref is the slice of *ant.Engine the console renders. Keeping it an interface
// makes the coupling explicit and lets the route tests run against a fake with
// no network (8000_ant_serve §6.2, §17).
type Deref interface {
	Domains() []ant.DomainInfo
	Domain(scheme string) (ant.DomainInfo, bool)
	Resolve(input, on string) (kit.URI, error)
	URL(u kit.URI) (string, error)
	Get(ctx context.Context, u kit.URI) (kit.Envelope, error)
	Dereference(ctx context.Context, u kit.URI, refresh bool) (ant.Fetched, error)
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

// Build is the binary's release identity, surfaced on the About page and used to
// cache-bust the embedded assets.
type Build struct {
	Version string
	Commit  string
	Date    string
}

// Console renders the web surface over a Deref.
type Console struct {
	e      Deref
	build  Build
	tpl    map[string]*template.Template // page name -> base+partials+page
	assets http.Handler                  // static file server over the embedded FS
}

// pages are the page templates under templates/pages; each is parsed together
// with the shell and the partials into its own set (8000_ant_serve §6.3).
var pages = []string{
	"dashboard", "resource", "collection", "search", "links", "resolve",
	"locate", "graph", "browse", "domain", "about", "error", "notfound",
}

// New parses every template against the embedded FS and returns a ready Console.
// Parsing once at construction means no per-request template work.
func New(e Deref, b Build) (*Console, error) {
	c := &Console{e: e, build: b, tpl: map[string]*template.Template{}}
	for _, name := range pages {
		t, err := template.New("base.html").Funcs(c.funcs()).ParseFS(files,
			"templates/base.html",
			"templates/partials/*.html",
			"templates/pages/"+name+".html",
		)
		if err != nil {
			return nil, err
		}
		c.tpl[name] = t
	}
	sub, err := fs.Sub(files, "assets")
	if err != nil {
		return nil, err
	}
	c.assets = http.StripPrefix("/assets/", cacheForever(http.FileServer(http.FS(sub))))
	return c, nil
}

// Handler returns the console's HTTP handler. It routes by the first path
// segment rather than through http.ServeMux on purpose: a resource URI in the
// path carries a "//" (GET /x://status/20) and ServeMux's path cleaning would
// 301-redirect it before any handler ran. Dispatching on the first segment leaves
// the raw path untouched (the regression serve_test.go guards).
func (c *Console) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := newNonce()
		secureHeaders(w, nonce)
		r = r.WithContext(context.WithValue(r.Context(), nonceKey{}, nonce))
		c.route(w, r)
	})
}

// route dispatches a request by its first path segment. It is split out of
// Handler so the /api facade can rewrite a request to force JSON and re-enter the
// same routing without duplicating it.
func (c *Console) route(w http.ResponseWriter, r *http.Request) {
	raw := trimLeadingSlash(r.URL.Path)
	switch seg := firstSegment(raw); seg {
	case "":
		c.home(w, r)
	case "assets":
		c.assets.ServeHTTP(w, r)
	case "healthz":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	case "api":
		c.api(w, r)
	case "view":
		c.resource(w, r, r.URL.Query().Get("uri"))
	case "ls":
		c.collection(w, r)
	case "search":
		c.search(w, r)
	case "links":
		c.linksPage(w, r)
	case "resolve":
		c.resolve(w, r)
	case "url":
		c.locate(w, r)
	case "graph":
		c.graph(w, r)
	case "browse":
		c.browse(w, r)
	case "domain":
		c.domainPage(w, r)
	case "about":
		c.about(w, r)
	case "export":
		c.export(w, r)
	default:
		if isSchemeSegment(seg) {
			c.resource(w, r, raw) // raw-URI dereference: /goodreads://book/1
			return
		}
		c.notFound(w, r)
	}
}

// api is the explicit JSON facade: GET /api/<route> serves the same data as
// /<route> but always as JSON, for scripts that would rather not send an Accept
// header. It strips the /api prefix, pins format=json, and re-enters the router;
// wantsJSON then returns true for every downstream handler.
func (c *Console) api(w http.ResponseWriter, r *http.Request) {
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/" + strings.TrimPrefix(trimLeadingSlash(r.URL.Path), "api/")
	q := r2.URL.Query()
	q.Set("format", "json")
	r2.URL.RawQuery = q.Encode()
	c.route(w, r2)
}
