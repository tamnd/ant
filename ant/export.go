package ant

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tamnd/any-cli/kit"
)

// dataFile is the on-disk path a URI's record is written to: the data root
// joined with the URI's data path and an extension. It is the single rule of
// 8000_uri §6.1, "the file path is the URI".
func (e *Engine) dataFile(u kit.URI, ext string) string {
	return filepath.Join(e.root, filepath.FromSlash(u.DataPath())) + "." + ext
}

// ExportReport is the honest accounting of an export walk: what was written, what
// was already present, and which URIs failed (so a partial export never reads as
// a complete one).
type ExportReport struct {
	Root    string            `json:"root"`
	Written []string          `json:"written"`
	Skipped []string          `json:"skipped,omitempty"`
	Errors  map[string]string `json:"errors,omitempty"`
}

// Export materializes a URI's record to the data tree and, with follow > 0,
// walks its links to that depth, writing each record under its own URI path. It
// deduplicates by canonical URI so a cycle terminates, and reports every outcome
// rather than failing the whole walk on one bad node. asMarkdown also writes a
// .md body file for records that have one.
func (e *Engine) Export(ctx context.Context, root kit.URI, follow int, asMarkdown bool) (*ExportReport, error) {
	rep := &ExportReport{Root: e.root, Errors: map[string]string{}}
	seen := map[string]bool{}

	type item struct {
		u     kit.URI
		depth int
	}
	queue := []item{{root, 0}}

	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		key := it.u.String()
		if seen[key] {
			continue
		}
		seen[key] = true

		env, err := e.Get(ctx, it.u)
		if err != nil {
			rep.Errors[key] = err.Error()
			continue
		}
		paths, err := e.writeEnvelope(it.u, env, asMarkdown)
		if err != nil {
			rep.Errors[key] = err.Error()
			continue
		}
		rep.Written = append(rep.Written, paths...)

		if it.depth >= follow {
			continue
		}
		for _, lu := range e.host.Links(env.Data) {
			if !seen[lu.String()] {
				queue = append(queue, item{lu, it.depth + 1})
			}
		}
	}
	if len(rep.Errors) == 0 {
		rep.Errors = nil
	}
	return rep, nil
}

// writeEnvelope writes the JSON record (always) and, when asMarkdown and the
// record carries a body, a Markdown file with a JSON front-matter header. It
// returns the paths written.
func (e *Engine) writeEnvelope(u kit.URI, env kit.Envelope, asMarkdown bool) ([]string, error) {
	jsonPath := e.dataFile(u, "json")
	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
		return nil, err
	}
	blob, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(jsonPath, append(blob, '\n'), 0o644); err != nil {
		return nil, err
	}
	// Keep the in-memory LL index consistent with what just hit disk, so a record
	// fetched mid-session shows up in browse without a re-walk. Derive the URI from
	// the path so it matches the exact string form LL stores.
	if uri, ok := e.pathToURI(jsonPath); ok {
		e.indexAdd(uri)
	}
	written := []string{jsonPath}

	if asMarkdown {
		if body, ok := e.host.Body(env.Data); ok {
			mdPath := e.dataFile(u, "md")
			header, err := json.MarshalIndent(map[string]any{
				"@id": env.ID, "@type": env.Type, "@fetched": env.Fetched,
			}, "", "  ")
			if err != nil {
				return written, err
			}
			doc := "---\n" + string(header) + "\n---\n\n" + body + "\n"
			if err := os.WriteFile(mdPath, []byte(doc), 0o644); err != nil {
				return written, err
			}
			written = append(written, mdPath)
		}
	}
	return written, nil
}

// Import reads an exported record file back into its envelope, the inverse of
// Export and the home of `ant import`.
func (e *Engine) Import(path string) (map[string]any, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var env map[string]any
	if err := json.Unmarshal(blob, &env); err != nil {
		return nil, fmt.Errorf("%s: not a JSON record: %w", path, err)
	}
	return env, nil
}

// LL lists the record URIs already materialized on disk under a URI prefix. An
// empty prefix lists the whole tree. The first call for a prefix walks the
// filesystem; the result is held in an in-memory index, and every later
// cache-write folds the new URI into the matching listings (indexAdd), so a
// repeat call returns from memory without touching disk. This is what keeps the
// web console's dashboard and browse pages fast as the data tree grows. A
// long-lived process (ant serve) is the only writer of its own tree, so the
// index stays consistent for the session; an external write lands on the next
// restart.
func (e *Engine) LL(prefix string) ([]string, error) {
	key := canonPrefix(prefix)
	e.llMu.RLock()
	if uris, ok := e.llCache[key]; ok {
		e.llMu.RUnlock()
		return uris, nil
	}
	e.llMu.RUnlock()

	uris, err := e.scanLL(prefix)
	if err != nil {
		return nil, err
	}
	e.llMu.Lock()
	e.llCache[key] = uris
	e.llMu.Unlock()
	return uris, nil
}

// scanLL is the filesystem walk behind LL: it lists every .json record under the
// prefix's data path. It is pure filesystem work, offline, and runs once per
// prefix before the in-memory index serves the rest.
func (e *Engine) scanLL(prefix string) ([]string, error) {
	sub := prefixDir(prefix)
	dir := filepath.Join(e.root, filepath.FromSlash(sub))

	// A prefix may name a directory or a specific record stem; walk the nearest
	// existing directory and filter by the prefix's data path.
	walkRoot := dir
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		walkRoot = filepath.Dir(dir)
	}
	if _, err := os.Stat(walkRoot); err != nil {
		return nil, nil // nothing exported yet under this prefix
	}

	var out []string
	err := filepath.WalkDir(walkRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".json") {
			return nil
		}
		uri, ok := e.pathToURI(p)
		if !ok {
			return nil
		}
		if prefix == "" || strings.HasPrefix(uri, canonPrefix(prefix)) {
			out = append(out, uri)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// indexAdd folds a freshly written URI into every cached LL listing it belongs
// to, keeping the in-memory index warm and consistent after a cache-write
// without re-walking the tree. It mirrors LL's own prefix filter so the index
// matches what a fresh walk would return.
func (e *Engine) indexAdd(uri string) {
	e.llMu.Lock()
	defer e.llMu.Unlock()
	for key, uris := range e.llCache {
		if key != "" && !strings.HasPrefix(uri, canonPrefix(key)) {
			continue
		}
		i := sort.SearchStrings(uris, uri)
		if i < len(uris) && uris[i] == uri {
			continue // already indexed
		}
		uris = append(uris, "")
		copy(uris[i+1:], uris[i:])
		uris[i] = uri
		e.llCache[key] = uris
	}
}

// pathToURI reverses dataFile: it turns an on-disk record path back into its
// canonical URI string. It returns false for a path outside the data root.
func (e *Engine) pathToURI(p string) (string, bool) {
	rel, err := filepath.Rel(e.root, p)
	if err != nil {
		return "", false
	}
	rel = strings.TrimSuffix(filepath.ToSlash(rel), ".json")
	scheme, rest, ok := strings.Cut(rel, "/")
	if !ok {
		return "", false
	}
	authority, id, ok := strings.Cut(rest, "/")
	if !ok {
		return "", false
	}
	return scheme + "://" + authority + "/" + id, true
}

// GraphNode is one resource in a walked subgraph.
type GraphNode struct {
	URI  string `json:"uri"`
	Type string `json:"type"`
}

// GraphEdge is one typed link from a resource to another.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Graph is the subgraph `ant graph` walks: every node reached within the depth
// and every edge between them.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Walk explores the link graph from root to the given depth, fetching each node
// once and recording its outbound edges. A failed fetch is skipped (its edges
// are simply absent) so one dead link does not abort the walk.
func (e *Engine) Walk(ctx context.Context, root kit.URI, depth int) (*Graph, error) {
	g := &Graph{}
	seen := map[string]bool{}
	nodeSeen := map[string]bool{}

	addNode := func(uri, typ string) {
		if !nodeSeen[uri] {
			nodeSeen[uri] = true
			g.Nodes = append(g.Nodes, GraphNode{URI: uri, Type: typ})
		}
	}

	type item struct {
		u kit.URI
		d int
	}
	queue := []item{{root, 0}}
	for len(queue) > 0 {
		it := queue[0]
		queue = queue[1:]
		key := it.u.String()
		if seen[key] {
			continue
		}
		seen[key] = true

		env, err := e.Get(ctx, it.u)
		if err != nil {
			addNode(key, it.u.Scheme+"/"+it.u.Authority)
			continue
		}
		addNode(env.ID, env.Type)
		if it.d >= depth {
			continue
		}
		for _, lu := range e.host.Links(env.Data) {
			g.Edges = append(g.Edges, GraphEdge{From: env.ID, To: lu.String()})
			addNode(lu.String(), lu.Scheme+"/"+lu.Authority)
			if !seen[lu.String()] {
				queue = append(queue, item{lu, it.d + 1})
			}
		}
	}
	return g, nil
}

// Dot renders a graph as Graphviz DOT.
func (g *Graph) Dot() string {
	var b strings.Builder
	b.WriteString("digraph ant {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box, fontname=\"monospace\"];\n")
	for _, n := range g.Nodes {
		fmt.Fprintf(&b, "  %q;\n", n.URI)
	}
	for _, ed := range g.Edges {
		fmt.Fprintf(&b, "  %q -> %q;\n", ed.From, ed.To)
	}
	b.WriteString("}\n")
	return b.String()
}

// prefixDir turns a URI prefix ("goodreads://book/") into the data-tree
// subpath it names ("goodreads/book"), tolerating a missing or partial form.
func prefixDir(prefix string) string {
	p := canonPrefix(prefix)
	p = strings.Replace(p, "://", "/", 1)
	return strings.Trim(p, "/")
}

// canonPrefix normalizes a URI prefix for comparison: trims spaces and a single
// trailing slash that a user adds to mean "everything under here".
func canonPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	return prefix
}
