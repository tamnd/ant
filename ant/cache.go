package ant

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/tamnd/any-cli/kit"
)

// Fetched is a dereferenced record with the provenance the web console needs:
// whether it came from the on-disk cache or a live fetch, its long-text body
// when it has one, and the canonical envelope JSON, so a renderer can show the
// record's fields in their declared order (a map would lose it).
type Fetched struct {
	Env       kit.Envelope
	Raw       json.RawMessage // the indented envelope JSON, as written to disk
	Body      string
	HasBody   bool
	FromCache bool
}

// Dereference resolves a URI cache-first: it returns the record already
// materialized under the data tree when one is present, and only fetches from
// the network on a cache miss or when refresh forces it. A live fetch is written
// back to the tree (JSON always, plus Markdown when the record has a body) so the
// next read is offline. This is the read path the web console drives, so browsing
// never re-fetches what ant already holds, and the refresh switch is the explicit
// way to pull a fresh copy.
func (e *Engine) Dereference(ctx context.Context, u kit.URI, refresh bool) (Fetched, error) {
	if !refresh {
		if f, ok := e.readCache(u); ok {
			return f, nil
		}
	}
	env, err := e.Get(ctx, u)
	if err != nil {
		return Fetched{}, err
	}
	body, hasBody := e.host.Body(env.Data)
	// Write the record back so the next read is a cache hit. A write failure must
	// not fail the read: the record is already in hand, and a read-only data dir
	// should still serve.
	_, _ = e.writeEnvelope(u, env, hasBody)
	raw, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return Fetched{}, err
	}
	return Fetched{Env: env, Raw: raw, Body: body, HasBody: hasBody, FromCache: false}, nil
}

// Cached reports whether a URI's record is already materialized on disk, so a
// caller can show a cache badge or a refresh affordance without reading the file.
func (e *Engine) Cached(u kit.URI) bool {
	_, err := os.Stat(e.dataFile(u, "json"))
	return err == nil
}

// Lookup returns a record from the on-disk cache without ever touching the
// network, so the web console can render a cached page instantly and route only a
// miss to a background fetch. ok is false on a miss (absent or unreadable). It is
// the read-only half of Dereference: same cache read, no write-back, no fetch.
func (e *Engine) Lookup(u kit.URI) (Fetched, bool) {
	return e.readCache(u)
}

// readCache reads a materialized record from the data tree, returning false on
// any miss (absent or unreadable) so the caller falls through to a live fetch.
func (e *Engine) readCache(u kit.URI) (Fetched, bool) {
	blob, err := os.ReadFile(e.dataFile(u, "json"))
	if err != nil {
		return Fetched{}, false
	}
	var env kit.Envelope
	if err := json.Unmarshal(blob, &env); err != nil {
		return Fetched{}, false
	}
	f := Fetched{Env: env, Raw: blob, FromCache: true}
	if body, ok := readBodyFile(e.dataFile(u, "md")); ok {
		f.Body, f.HasBody = body, true
	}
	return f, true
}

// readBodyFile reads an exported Markdown body, stripping the JSON front-matter
// block writeEnvelope writes between the leading "---" fences.
func readBodyFile(path string) (string, bool) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	s := string(blob)
	if strings.HasPrefix(s, "---\n") {
		if i := strings.Index(s[4:], "\n---\n"); i >= 0 {
			s = s[4+i+len("\n---\n"):]
		}
	}
	return strings.TrimLeft(s, "\n"), true
}
