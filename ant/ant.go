// Package ant is the library behind the ant command line: a thin layer over a
// kit.Host that dereferences a resource URI to a record, regardless of which
// site it lives on.
//
// ant owns no HTTP client and no per-site knowledge. Every domain it can address
// registers itself with kit from its own package's init, exactly as a
// database/sql driver does, and a program enables a domain with a single blank
// import:
//
//	import (
//		_ "github.com/tamnd/goodread-cli/goodread"
//		_ "github.com/tamnd/x-cli/x"
//	)
//
// The Engine here is the sql.DB analogue: it opens the host once, then answers
// get/ls/links/resolve/url/export over the whole namespace.
package ant

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tamnd/any-cli/kit"
)

// Engine dereferences resource URIs across every registered domain and
// materializes them to the on-disk URI tree. It is safe for concurrent use; the
// underlying host builds each domain's client lazily on first touch.
type Engine struct {
	host *kit.Host
	root string           // the data tree root ($HOME/data, ANT_DATA-overridable)
	now  func() time.Time // the fetch clock, injectable so tests are deterministic

	// llMu guards llCache, the in-memory index of materialized URIs keyed by the
	// listing prefix. A directory walk runs once per prefix; every cache-write
	// folds the new URI into the matching listings, so repeat reads (the web
	// console's dashboard and browse pages) never re-walk the tree. See LL.
	llMu    sync.RWMutex
	llCache map[string][]string
}

// Option customizes an Engine at New.
type Option func(*Engine)

// WithRoot sets the on-disk data tree root (the default is $HOME/data).
func WithRoot(dir string) Option { return func(e *Engine) { e.root = dir } }

// WithClock sets the clock used to stamp @fetched, so a test can pin it.
func WithClock(fn func() time.Time) Option { return func(e *Engine) { e.now = fn } }

// New opens the host over every registered domain and returns a ready Engine.
func New(opts ...Option) (*Engine, error) {
	h, err := kit.Open()
	if err != nil {
		return nil, err
	}
	e := &Engine{host: h, now: time.Now, llCache: map[string][]string{}}
	for _, o := range opts {
		o(e)
	}
	if e.root == "" {
		e.root = defaultRoot()
	}
	return e, nil
}

// Root returns the data tree root the Engine writes under.
func (e *Engine) Root() string { return e.root }

// WarmIndex pre-populates the in-memory LL index for every registered domain, so
// the first browse or dashboard request is served from memory rather than paying
// for a cold filesystem walk. It walks only ant's own domain subtrees, never the
// whole shared data root. A long-lived process (ant serve) calls this once in the
// background at startup; it is a no-op to call again.
func (e *Engine) WarmIndex() {
	for _, scheme := range e.host.Domains() {
		_, _ = e.LL(scheme + "://")
	}
}

// Domains returns the registered domains the Engine can address, sorted by
// scheme. It is the analogue of sql.Drivers and backs `ant domains`.
func (e *Engine) Domains() []DomainInfo {
	var out []DomainInfo
	for _, scheme := range e.host.Domains() {
		if info, ok := e.host.Domain(scheme); ok {
			out = append(out, DomainInfo{
				Scheme:  scheme,
				Aliases: info.Aliases,
				Hosts:   info.Hosts,
				Binary:  info.Identity.Binary,
				Short:   info.Identity.Short,
				Site:    info.Identity.Site,
				Repo:    info.Identity.Repo,
			})
		}
	}
	return out
}

// Domain returns the descriptor of a single registered domain by scheme or
// alias, the lookup the web console uses to render one domain's detail page.
func (e *Engine) Domain(scheme string) (DomainInfo, bool) {
	info, ok := e.host.Domain(scheme)
	if !ok {
		return DomainInfo{}, false
	}
	return DomainInfo{
		Scheme:  info.Scheme,
		Aliases: info.Aliases,
		Hosts:   info.Hosts,
		Binary:  info.Identity.Binary,
		Short:   info.Identity.Short,
		Site:    info.Identity.Site,
		Repo:    info.Identity.Repo,
	}, true
}

// DomainInfo is one registered domain, as `ant domains` prints it.
type DomainInfo struct {
	Scheme  string   `json:"scheme"`
	Aliases []string `json:"aliases,omitempty"`
	Hosts   []string `json:"hosts,omitempty"`
	Binary  string   `json:"binary,omitempty"`
	Short   string   `json:"short,omitempty"`
	Site    string   `json:"site,omitempty"`
	Repo    string   `json:"repo,omitempty"`
}

// Resolve normalizes any input into a canonical URI. With on set, it resolves
// the input within that domain (the home of bare ids and @handles); otherwise it
// accepts a resource URI or a site URL whose host a domain claims.
func (e *Engine) Resolve(input, on string) (kit.URI, error) {
	if on != "" {
		return e.host.ResolveOn(on, input)
	}
	return e.host.Resolve(input)
}

// URL returns the live https location of a URI, the inverse of Resolve.
func (e *Engine) URL(u kit.URI) (string, error) { return e.host.Locate(u) }

// Get dereferences a URI to its record, wrapped in the self-describing envelope
// (@id/@type/@fetched/@links) the data surface uses.
func (e *Engine) Get(ctx context.Context, u kit.URI) (kit.Envelope, error) {
	rec, err := e.host.Get(ctx, u)
	if err != nil {
		return kit.Envelope{}, err
	}
	return e.host.Wrap(rec, e.now())
}

// List returns the member records of a collection URI, each wrapped as an
// envelope. limit caps the result (0 means the op's own default).
func (e *Engine) List(ctx context.Context, u kit.URI, limit int) ([]kit.Envelope, error) {
	recs, err := e.host.List(ctx, u, limit)
	if err != nil {
		return nil, err
	}
	out := make([]kit.Envelope, 0, len(recs))
	for _, rec := range recs {
		env, err := e.host.Wrap(rec, e.now())
		if err != nil {
			return nil, err
		}
		out = append(out, env)
	}
	return out, nil
}

// Searchable reports whether a domain (by scheme or alias) supports free-text
// search, so the web console can decide to show a search box for it.
func (e *Engine) Searchable(scheme string) bool { return e.host.Searchable(scheme) }

// Search runs a domain's free-text search and returns the hits as envelopes. A
// hit that is URI-addressable carries its canonical @id, so it links straight to
// get; one that is not still surfaces, wrapped with the scheme as @type and no
// @id. limit caps the result (0 means the op's own default). Search hits are
// previews and are not written to the data tree; dereferencing one caches it.
func (e *Engine) Search(ctx context.Context, scheme, query string, limit int) ([]kit.Envelope, error) {
	recs, err := e.host.Search(ctx, scheme, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]kit.Envelope, 0, len(recs))
	for _, rec := range recs {
		env, err := e.host.Wrap(rec, e.now())
		if err != nil {
			env = kit.Envelope{Type: scheme, Data: rec}
		}
		// A search hit often is not itself a mintable resource (it is a preview
		// shape, not the record type), so Wrap leaves @id empty. When the hit
		// carries a site URL, resolve it back to the canonical URI so the result is
		// still one click from its record.
		if env.ID == "" {
			if u, ok := e.uriFromHit(scheme, rec); ok {
				env.ID = u.String()
				env.Type = u.Scheme + "/" + u.Authority
			}
		}
		out = append(out, env)
	}
	return out, nil
}

// uriFromHit recovers a canonical URI from a search hit that did not mint one, by
// resolving a URL-bearing field (url/link/href) through the domain. It is how a
// preview-shaped result becomes dereferenceable.
func (e *Engine) uriFromHit(scheme string, rec any) (kit.URI, bool) {
	blob, err := json.Marshal(rec)
	if err != nil {
		return kit.URI{}, false
	}
	var fields map[string]any
	if err := json.Unmarshal(blob, &fields); err != nil {
		return kit.URI{}, false
	}
	for _, key := range []string{"url", "link", "href", "permalink"} {
		s, ok := fields[key].(string)
		if !ok || s == "" {
			continue
		}
		if u, err := e.Resolve(s, ""); err == nil {
			return u, true
		}
		if u, err := e.Resolve(s, scheme); err == nil {
			return u, true
		}
	}
	return kit.URI{}, false
}

// Links fetches a URI's record and returns its outbound graph edges as URIs.
func (e *Engine) Links(ctx context.Context, u kit.URI) ([]kit.URI, error) {
	rec, err := e.host.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	return e.host.Links(rec), nil
}

// BodyOf returns the long-text body of an already-fetched envelope's record,
// so `ant get -o md` need not fetch twice.
func (e *Engine) BodyOf(env kit.Envelope) (string, bool) {
	return e.host.Body(env.Data)
}

// Body fetches a URI's record and returns its long-text body, when it has one.
func (e *Engine) Body(ctx context.Context, u kit.URI) (string, bool, error) {
	rec, err := e.host.Get(ctx, u)
	if err != nil {
		return "", false, err
	}
	body, ok := e.host.Body(rec)
	return body, ok, nil
}

// defaultRoot is the family's shared data root: ANT_DATA, then $HOME/data
// (8000_uri §6.1).
func defaultRoot() string {
	if v := os.Getenv("ANT_DATA"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "data")
	}
	return filepath.Join(home, "data")
}
