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
	"os"
	"path/filepath"
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
	e := &Engine{host: h, now: time.Now}
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
