package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// markdown renders a record's body. Raw HTML in the source is escaped (WithUnsafe
// is never set), so untrusted prose cannot inject markup (8000_ant_serve §8.3).
var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

// View is the data every page renders against: the shell needs the sidebar
// domains, the CSP nonce, the asset version, and the build; the page reads Page.
type View struct {
	Title   string
	Active  string // the nav key to mark current
	Nonce   string
	AssetV  string
	Version string
	Domains []domainNav
	Page    any
}

// domainNav is one sidebar entry.
type domainNav struct {
	Scheme  string
	Aliases []string
	Accent  string
	Href    string
	Search  bool // domain registered a free-text search op
}

// view assembles the common shell data plus the page-specific payload.
func (c *Console) view(r *http.Request, title, active string, page any) View {
	nonce, _ := r.Context().Value(nonceKey{}).(string)
	var nav []domainNav
	for _, d := range c.e.Domains() {
		nav = append(nav, domainNav{
			Scheme:  d.Scheme,
			Aliases: d.Aliases,
			Accent:  accent(d.Scheme),
			Href:    domainHref(d.Scheme),
			Search:  c.e.Searchable(d.Scheme),
		})
	}
	return View{
		Title:   title,
		Active:  active,
		Nonce:   nonce,
		AssetV:  c.assetV(),
		Version: c.build.Version,
		Domains: nav,
		Page:    page,
	}
}

// render executes a page template into a buffer first, so a template error
// becomes a clean 500 rather than a half-written response.
func (c *Console) render(w http.ResponseWriter, r *http.Request, status int, name, title, active string, page any) {
	t, ok := c.tpl[name]
	if !ok {
		http.Error(w, "unknown page: "+name, http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base.html", c.view(r, title, active, page)); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// errView is the model for the shared error page.
type errView struct {
	Status  int
	Kind    string
	Title   string
	Message string
	URI     string // the URI in play, if any
	LiveURL string // the live source URL, offered as a manual escape hatch
	Suggest []suggestion
}

type suggestion struct {
	Label string
	Href  string
}

// renderError maps an engine error to the styled error page (8000_ant_serve §12).
func (c *Console) renderError(w http.ResponseWriter, r *http.Request, err error, uri string) {
	status := statusFor(err)
	ev := errView{
		Status:  status,
		Kind:    kindName(status),
		Title:   errTitle(status, uri),
		Message: err.Error(),
		URI:     uri,
	}
	if uri != "" {
		if u, perr := kit.ParseURI(uri); perr == nil {
			if loc, lerr := c.e.URL(u); lerr == nil {
				ev.LiveURL = loc
			}
			ev.Suggest = append(ev.Suggest,
				suggestion{"View as a collection", lsHref(uri)},
				suggestion{"Walk the graph", graphHref(uri, 1)},
			)
		}
	}
	ev.Suggest = append(ev.Suggest, suggestion{"All domains", "/about"})
	c.render(w, r, status, "error", ev.Title, "", ev)
}

// funcs is the template function map (8000_ant_serve §6.3, §8).
func (c *Console) funcs() template.FuncMap {
	return template.FuncMap{
		"viewHref":    viewHref,
		"lsHref":      lsHref,
		"linksHref":   linksHrefOf,
		"graphHref":   func(u string) string { return graphHref(u, 1) },
		"domainHref":  domainHref,
		"urlHref":     urlHrefOf,
		"browseHref":  browseHref,
		"resolveHref": resolveHref,
		"searchHref":  searchHref,
		"accent":      accent,
		"humanize":    humanize,
		"relTime":     relTime,
		"value":       func(field string, v any) template.HTML { return valueHTML(field, v, 0) },
		"body":        bodyHTML,
		"prettyJSON":  prettyJSON,
		"examples":    exampleURIs,
		"shortID":     shortID,
		"isImage":     func(field, s string) bool { return isImageField(field) && isHTTPURL(s) },
		"dict":        dict,
		"add":         func(a, b int) int { return a + b },
	}
}

// assetV is the asset cache-buster: the short commit, or the version.
func (c *Console) assetV() string {
	if c.build.Commit != "" && c.build.Commit != "none" {
		return c.build.Commit
	}
	if c.build.Version != "" {
		return c.build.Version
	}
	return "dev"
}

// --- href builders (shared by templates and the value renderer) -------------

func viewHref(uri string) string    { return "/view?uri=" + url.QueryEscape(uri) }
func lsHref(uri string) string      { return "/ls?uri=" + url.QueryEscape(uri) }
func linksHrefOf(uri string) string { return "/links?uri=" + url.QueryEscape(uri) }
func urlHrefOf(uri string) string   { return "/url?uri=" + url.QueryEscape(uri) }
func browseHref(prefix string) string {
	if prefix == "" {
		return "/browse"
	}
	return "/browse?prefix=" + url.QueryEscape(prefix)
}
func domainHref(scheme string) string { return "/domain?scheme=" + url.QueryEscape(scheme) }
func graphHref(uri string, depth int) string {
	return "/graph?uri=" + url.QueryEscape(uri) + "&depth=" + strconv.Itoa(depth)
}
func resolveHref(input, on string) string {
	q := url.Values{}
	q.Set("input", input)
	if on != "" {
		q.Set("on", on)
	}
	return "/resolve?" + q.Encode()
}

func searchHref(scheme, q string) string {
	v := url.Values{}
	if scheme != "" {
		v.Set("on", scheme)
	}
	if q != "" {
		v.Set("q", q)
	}
	if len(v) == 0 {
		return "/search"
	}
	return "/search?" + v.Encode()
}

// --- breadcrumbs and the data-tree prefix model -----------------------------

// crumb is one breadcrumb: a label, the browse href it points at, and whether it
// is the current (last) node, which the template renders as plain text.
type crumb struct {
	Label string
	Href  string
	Last  bool
}

// splitPrefix turns a browse prefix string ("", "goodreads://",
// "goodreads://book") into its data-tree segments ([], [goodreads],
// [goodreads book]). It is the inverse of joinPrefix.
func splitPrefix(prefix string) []string {
	p := strings.TrimSpace(prefix)
	if p == "" {
		return nil
	}
	p = strings.Replace(p, "://", "/", 1)
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// joinPrefix renders data-tree segments back into a browse prefix string: a lone
// scheme becomes "scheme://" so it reads as a domain root, deeper segments append
// under it.
func joinPrefix(segs []string) string {
	switch len(segs) {
	case 0:
		return ""
	case 1:
		return segs[0] + "://"
	default:
		return segs[0] + "://" + strings.Join(segs[1:], "/")
	}
}

// uriSegs splits a record URI into its data-tree segments [scheme, authority,
// id...], the same shape splitPrefix yields, so the two can be compared.
func uriSegs(uri string) []string {
	u, err := kit.ParseURI(uri)
	if err != nil {
		return nil
	}
	return append([]string{u.Scheme, u.Authority}, u.Path...)
}

// segsHasPrefix reports whether segs begins with pre.
func segsHasPrefix(segs, pre []string) bool {
	if len(segs) < len(pre) {
		return false
	}
	for i, p := range pre {
		if segs[i] != p {
			return false
		}
	}
	return true
}

// crumbsForPrefix builds the breadcrumb trail for a data-tree node, rooted at the
// data tree itself. Each crumb but the last links to its browse view; the last is
// the current node.
func crumbsForPrefix(segs []string) []crumb {
	out := []crumb{{Label: "data", Href: browseHref("")}}
	for i := range segs {
		out = append(out, crumb{Label: segs[i], Href: browseHref(joinPrefix(segs[:i+1]))})
	}
	out[len(out)-1].Last = true
	return out
}

// crumbsFor is the breadcrumb trail to a record, so the resource page shows where
// the record sits in the tree and every ancestor is one click away.
func crumbsFor(u kit.URI) []crumb {
	return crumbsForPrefix(append([]string{u.Scheme, u.Authority}, u.Path...))
}

// --- value rendering --------------------------------------------------------

// kvPair preserves the field order of a record's JSON object (a map would lose
// it), so the data card shows fields in the order the driver declared them.
type kvPair struct {
	Key string
	Val any
}

type orderedObj []kvPair

// decodeOrdered reads JSON into an order-preserving tree: objects become
// orderedObj, arrays []any, numbers json.Number, the rest their natural types.
func decodeOrdered(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch delim {
	case '{':
		obj := orderedObj{}
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			val, err := decodeOrdered(dec)
			if err != nil {
				return nil, err
			}
			obj = append(obj, kvPair{Key: keyTok.(string), Val: val})
		}
		_, err = dec.Token() // closing '}'
		return obj, err
	case '[':
		arr := []any{}
		for dec.More() {
			val, err := decodeOrdered(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		_, err = dec.Token() // closing ']'
		return arr, err
	default:
		return nil, fmt.Errorf("unexpected delimiter %v", delim)
	}
}

// orderedData round-trips a record through JSON into an order-preserving tree, so
// the page shows exactly the fields the JSON API shows (8000_ant_serve §8.2).
func orderedData(rec any) orderedObj {
	blob, err := json.Marshal(rec)
	if err != nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(blob))
	dec.UseNumber()
	v, err := decodeOrdered(dec)
	if err != nil {
		return nil
	}
	if obj, ok := v.(orderedObj); ok {
		return obj
	}
	return nil
}

// orderedDataFromRaw pulls the "data" object out of an envelope's raw JSON,
// preserving the record's declared field order. The resource page reads from this
// rather than re-marshaling a decoded map, so a cache-loaded record shows its
// fields in the same order a freshly fetched one does (8000_ant_serve §23).
func orderedDataFromRaw(raw []byte) orderedObj {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := decodeOrdered(dec)
	if err != nil {
		return nil
	}
	env, ok := v.(orderedObj)
	if !ok {
		return nil
	}
	for _, kv := range env {
		if kv.Key == "data" {
			if d, ok := kv.Val.(orderedObj); ok {
				return d
			}
			return nil
		}
	}
	return nil
}

// valueHTML renders one record value to safe HTML. Every text path escapes its
// input; only this file mints template.HTML, and only after escaping
// (8000_ant_serve §8.1, §13).
func valueHTML(field string, v any, depth int) template.HTML {
	if depth > 8 {
		return template.HTML(`<span class="muted">…</span>`)
	}
	switch x := v.(type) {
	case nil:
		return template.HTML(`<span class="muted">—</span>`)
	case string:
		return stringHTML(field, x)
	case bool:
		return esc(strconv.FormatBool(x))
	case json.Number:
		return esc(groupNumber(x.String()))
	case float64:
		return esc(groupNumber(strconv.FormatFloat(x, 'f', -1, 64)))
	case orderedObj:
		if len(x) == 0 {
			return template.HTML(`<span class="muted">{}</span>`)
		}
		var b strings.Builder
		b.WriteString(`<div class="kv kv-nested">`)
		for _, kv := range x {
			b.WriteString(`<div class="kv-key">`)
			b.WriteString(html.EscapeString(humanize(kv.Key)))
			b.WriteString(`</div><div class="kv-val">`)
			b.WriteString(string(valueHTML(kv.Key, kv.Val, depth+1)))
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
		return template.HTML(b.String())
	case []any:
		if len(x) == 0 {
			return template.HTML(`<span class="muted">[]</span>`)
		}
		var b strings.Builder
		b.WriteString(`<ul class="vlist">`)
		for _, item := range x {
			b.WriteString(`<li>`)
			b.WriteString(string(valueHTML(field, item, depth+1)))
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
		return template.HTML(b.String())
	default:
		return esc(fmt.Sprint(x))
	}
}

// stringHTML renders a string value: a resource URI becomes an internal chip, an
// image-field URL becomes an <img>, any other URL an external link, and plain
// text is escaped (and clamped when very long).
func stringHTML(field, s string) template.HTML {
	if u, err := kit.ParseURI(s); err == nil && !kit.IsReservedKind(u.Scheme) {
		return template.HTML(`<a class="chip" href="` + html.EscapeString(viewHref(u.String())) +
			`"><span class="chip-scheme">` + html.EscapeString(u.Scheme) + `</span>` +
			html.EscapeString(u.Authority+"/"+u.ID()) + `</a>`)
	}
	if isHTTPURL(s) {
		if isImageField(field) {
			return template.HTML(`<img class="thumb" loading="lazy" src="` +
				html.EscapeString(s) + `" alt="">`)
		}
		return template.HTML(`<a class="ext" href="` + html.EscapeString(s) +
			`" target="_blank" rel="noopener noreferrer">` + html.EscapeString(truncate(s, 80)) +
			` ↗</a>`)
	}
	if len(s) > 400 {
		return template.HTML(`<div class="value-text clamp">` + html.EscapeString(s) + `</div>`)
	}
	return template.HTML(`<span class="value-text">` + html.EscapeString(s) + `</span>`)
}

// bodyHTML renders a record's body as safe Markdown.
func bodyHTML(text string) template.HTML {
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(text), &buf); err != nil {
		return template.HTML(`<pre>` + html.EscapeString(text) + `</pre>`)
	}
	return template.HTML(buf.String())
}

// prettyJSON marshals v indented; used for the raw-JSON disclosure.
func prettyJSON(v any) string {
	blob, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err.Error()
	}
	return string(blob)
}

// esc escapes text to HTML.
func esc(s string) template.HTML { return template.HTML(html.EscapeString(s)) }

// --- small helpers ----------------------------------------------------------

// accentHue maps a known scheme to a hue; an unknown driver hashes to a stable
// one so its dot and badge still get a consistent color (8000_ant_serve §3.1).
var accentHue = map[string]int{
	"goodreads": 32, "x": 202, "wikipedia": 222, "youtube": 0,
	"reddit": 16, "facebook": 221, "bilibili": 330, "amazon": 36,
	"archive": 150, "threads": 280, "douban": 130, "xiaohongshu": 348,
}

func accent(scheme string) string {
	h, ok := accentHue[scheme]
	if !ok {
		hh := fnv.New32a()
		_, _ = hh.Write([]byte(scheme))
		h = int(hh.Sum32() % 360)
	}
	return fmt.Sprintf("hsl(%d 72%% 52%%)", h)
}

// exampleURIs returns a couple of try-me URIs per known scheme for the cards and
// domain pages. Unknown drivers return none (better than a fabricated id).
var exampleURIs = func() func(string) []string {
	m := map[string][]string{
		"goodreads": {"goodreads://book/2767052", "goodreads://author/153394"},
		"x":         {"x://user/nasa", "x://status/20"},
		"wikipedia": {"wikipedia://page/Alan_Turing", "wikipedia://category/Computability_theory"},
		"youtube":   {"youtube://video/dQw4w9WgXcQ", "youtube://channel/UCuAXFkgsw1L7xaCfnd5JJOw"},
	}
	return func(scheme string) []string { return m[scheme] }
}()

// humanize turns a json field name into a label: similar_books -> "Similar books".
func humanize(name string) string {
	if name == "" {
		return name
	}
	s := strings.NewReplacer("_", " ", "-", " ").Replace(name)
	return strings.ToUpper(s[:1]) + s[1:]
}

// relTime renders an RFC3339 timestamp as a relative string; non-times pass back.
func relTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return t.Format("2006-01-02 15:04")
	}
}

// shortID is the last path segment of a URI, for compact graph/card labels.
func shortID(uri string) string {
	u, err := kit.ParseURI(uri)
	if err != nil {
		return uri
	}
	if len(u.Path) == 0 {
		return u.Authority
	}
	return u.Path[len(u.Path)-1]
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

var imageFields = map[string]bool{
	"thumbnail": true, "thumbnails": true, "image": true, "images": true,
	"avatar": true, "cover": true, "photo": true, "picture": true,
	"icon": true, "banner": true, "profile_image": true,
}

func isImageField(field string) bool { return imageFields[strings.ToLower(field)] }

// groupNumber inserts thousands separators into a plain integer string; it
// leaves decimals and non-numbers untouched.
func groupNumber(s string) string {
	neg := strings.HasPrefix(s, "-")
	d := strings.TrimPrefix(s, "-")
	if d == "" || strings.ContainsAny(d, ".eE") {
		return s
	}
	for _, r := range d {
		if r < '0' || r > '9' {
			return s
		}
	}
	if len(d) <= 4 {
		return s
	}
	var b strings.Builder
	for i, r := range d {
		if i > 0 && (len(d)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// dict builds a map from alternating key/value template args, for partials.
func dict(pairs ...any) map[string]any {
	m := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		k, _ := pairs[i].(string)
		m[k] = pairs[i+1]
	}
	return m
}

func kindName(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "Bad request"
	case http.StatusNotFound:
		return "Not found"
	case http.StatusBadGateway:
		return "Upstream error"
	default:
		return "Error"
	}
}

func errTitle(status int, uri string) string {
	switch status {
	case http.StatusBadRequest:
		return "That isn't a valid URI"
	case http.StatusNotFound:
		return "No such record"
	case http.StatusBadGateway:
		return "Couldn't reach the site"
	default:
		return "Something went wrong"
	}
}

// linkGroup is a record's @links for one source field, ready to render as chips.
type linkGroup struct {
	Field string
	URIs  []string
}

// linkGroupsOf turns the envelope's grouped links into a stable, sorted slice.
func linkGroupsOf(m map[string][]string) []linkGroup {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]linkGroup, 0, len(keys))
	for _, k := range keys {
		out = append(out, linkGroup{Field: k, URIs: m[k]})
	}
	return out
}

// graphPayload is what the graph page embeds for the canvas and the JSON API.
type graphPayload struct {
	Root  string          `json:"root"`
	Nodes []graphNodeOut  `json:"nodes"`
	Edges []ant.GraphEdge `json:"edges"`
}

type graphNodeOut struct {
	URI    string `json:"uri"`
	Type   string `json:"type"`
	Label  string `json:"label"`
	Accent string `json:"accent"`
}

// graphToPayload decorates the engine graph with per-node label and accent color.
func graphToPayload(root string, g *ant.Graph) graphPayload {
	p := graphPayload{Root: root, Edges: g.Edges}
	for _, n := range g.Nodes {
		scheme := n.Type
		if i := strings.IndexByte(scheme, '/'); i >= 0 {
			scheme = scheme[:i]
		}
		p.Nodes = append(p.Nodes, graphNodeOut{
			URI:    n.URI,
			Type:   n.Type,
			Label:  shortID(n.URI),
			Accent: accent(scheme),
		})
	}
	return p
}

// ctxValue is a tiny helper to read the request nonce in tests.
func ctxNonce(ctx context.Context) string {
	n, _ := ctx.Value(nonceKey{}).(string)
	return n
}
