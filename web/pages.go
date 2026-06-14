package web

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// exportTimeout bounds the one synchronous, state-changing action left on the
// request path: the export POST, which writes the data tree and follows links.
// Every read page is async (see jobs.go) and never blocks on a timeout; export is
// a deliberate button press, so a bounded wait with a clean error is the right DX
// (8000_ant_serve §24).
const exportTimeout = 60 * time.Second

// reqCtx derives a timeout-bounded context from the request so a hung upstream
// cannot pin the export forever, while a client disconnect still cancels it.
func reqCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), exportTimeout)
}

// --- dashboard / home -------------------------------------------------------

type dashView struct {
	Domains []domainCard
	Disk    diskSummary
}

type domainCard struct {
	Scheme   string
	Short    string
	Repo     string
	Accent   string
	Aliases  []string
	Hosts    []string
	Examples []string
}

type diskSummary struct {
	Root  string
	Count int
}

func (c *Console) home(w http.ResponseWriter, r *http.Request) {
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, map[string]any{"service": "ant", "domains": c.e.Domains()})
		return
	}
	// Count only ant's own domains, never the whole shared data root: $HOME/data
	// is home to many other tools' trees, so a whole-tree walk is both slow and
	// wrong (it would surface their files as ant records). Per-domain listings are
	// bounded and the in-memory index keeps them cheap.
	var cards []domainCard
	disk := diskSummary{Root: c.e.Root()}
	for _, d := range c.e.Domains() {
		cards = append(cards, domainCard{
			Scheme:   d.Scheme,
			Short:    d.Short,
			Repo:     d.Repo,
			Accent:   accent(d.Scheme),
			Aliases:  d.Aliases,
			Hosts:    d.Hosts,
			Examples: exampleURIs(d.Scheme),
		})
		if uris, err := c.e.LL(d.Scheme + "://"); err == nil {
			disk.Count += len(uris)
		}
	}
	c.render(w, r, http.StatusOK, "dashboard", "ant — every record is a URI", "home",
		dashView{Domains: cards, Disk: disk})
}

// --- resource (get) ---------------------------------------------------------

type resourceView struct {
	URI        string
	Scheme     string
	Authority  string
	ID         string
	Accent     string
	Type       string
	Fetched    string
	Fields     orderedObj
	Links      []linkGroup
	HasBody    bool
	Body       string // rendered by the body func in the template
	LiveURL    string
	RawJSON    string
	Crumbs     []crumb
	FromCache  bool
	Exported   bool
	RefreshURL string
}

func (c *Console) resource(w http.ResponseWriter, r *http.Request, raw string) {
	u, err := kit.ParseURI(raw)
	if err != nil {
		c.fail(w, r, errs.Usage("%s", err.Error()), raw)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"

	// Cache-first fast path: a materialized record renders instantly, with no
	// goroutine, no queue, and no network — the common case once a URI has been
	// seen. This is what keeps an already-fetched page well under the budget even
	// while a sibling tab waits on a slow site (8000_ant_serve §23, §24).
	if !refresh {
		if f, ok := c.e.Lookup(u); ok {
			c.emitResource(w, r, u, f)
			return
		}
	}

	// Miss (or forced refresh): run the fetch as a deduplicated background job so
	// the request never blocks on a slow upstream. A browser gets the grace race
	// (render inline if quick, loading screen if slow); a script waits for the data.
	jb := c.jobs.start(fetchKey("get", u.String(), 0, 0, refresh), func(ctx context.Context) (any, error) {
		return c.e.Dereference(ctx, u, refresh)
	})
	if wantsJSON(r) {
		ph, v, jerr := jb.wait(r.Context())
		switch ph {
		case jobReady:
			writeJSON(w, http.StatusOK, v.(ant.Fetched).Env)
		case jobError:
			writeJSONErr(w, jerr)
		default:
			writeJSON(w, http.StatusAccepted, pendingJSON(u))
		}
		return
	}
	ph, v, jerr := c.graced(r, jb)
	switch ph {
	case jobReady:
		c.emitResource(w, r, u, v.(ant.Fetched))
	case jobError:
		c.renderError(w, r, jerr, u.String())
	default:
		c.renderLoading(w, r, u, "record", "get", 0, 0, refresh)
	}
}

// emitResource renders a fetched record and warms its neighbors. Splitting it out
// lets the cache fast path and the background-job path render identically. The
// JSON branch returns before the prefetch, so only a browser view warms links.
func (c *Console) emitResource(w http.ResponseWriter, r *http.Request, u kit.URI, f ant.Fetched) {
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, f.Env)
		return
	}
	liveURL, _ := c.e.URL(u)
	rv := resourceView{
		URI:        f.Env.ID,
		Scheme:     u.Scheme,
		Authority:  u.Authority,
		ID:         u.ID(),
		Accent:     accent(u.Scheme),
		Type:       f.Env.Type,
		Fetched:    f.Env.Fetched,
		Fields:     orderedDataFromRaw(f.Raw),
		Links:      linkGroupsOf(f.Env.Links),
		HasBody:    f.HasBody,
		Body:       f.Body,
		LiveURL:    liveURL,
		RawJSON:    string(f.Raw),
		Crumbs:     crumbsFor(u),
		FromCache:  f.FromCache,
		Exported:   r.URL.Query().Get("exported") == "1",
		RefreshURL: viewHref(u.String()) + "&refresh=1",
	}
	c.render(w, r, http.StatusOK, "resource", f.Env.ID, "", rv)
	c.prefetchLinks(f.Env.Links)
}

// graced waits for a job up to the grace window, so a fast fetch renders inline
// and a slow one falls through to a loading screen rather than blocking the tab.
func (c *Console) graced(r *http.Request, jb *job) (jobPhase, any, error) {
	ctx, cancel := context.WithTimeout(r.Context(), graceWindow)
	defer cancel()
	return jb.wait(ctx)
}

// --- collection (ls) --------------------------------------------------------

type collectionView struct {
	URI    string
	N      int
	Accent string
	Cards  []recordCard
}

type recordCard struct {
	URI     string
	Title   string
	Snippet string
	Thumb   string
	Type    string
	Accent  string
}

func (c *Console) collection(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("uri")
	u, err := kit.ParseURI(raw)
	if err != nil {
		c.fail(w, r, errs.Usage("%s", err.Error()), raw)
		return
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	// A collection is always live (it is not cached as a unit), so it runs as a
	// background job: the page renders inline when the listing is quick and shows a
	// loading screen when the site is slow, never a timeout (8000_ant_serve §24).
	jb := c.jobs.start(fetchKey("ls", u.String(), n, 0, false), func(ctx context.Context) (any, error) {
		return c.e.List(ctx, u, n)
	})
	if wantsJSON(r) {
		ph, v, jerr := jb.wait(r.Context())
		switch ph {
		case jobReady:
			writeJSON(w, http.StatusOK, v.([]kit.Envelope))
		case jobError:
			writeJSONErr(w, jerr)
		default:
			writeJSON(w, http.StatusAccepted, pendingJSON(u))
		}
		return
	}
	ph, v, jerr := c.graced(r, jb)
	switch ph {
	case jobReady:
		cv := collectionView{URI: u.String(), N: n, Accent: accent(u.Scheme)}
		for _, env := range v.([]kit.Envelope) {
			cv.Cards = append(cv.Cards, cardFromEnv(env))
		}
		c.render(w, r, http.StatusOK, "collection", "Members of "+u.String(), "", cv)
	case jobError:
		c.renderError(w, r, jerr, u.String())
	default:
		c.renderLoading(w, r, u, "collection", "ls", n, 0, false)
	}
}

// cardFromEnv projects an envelope into a list card: a title, a one-line
// snippet, and a thumbnail, chosen from whichever common fields the record has.
func cardFromEnv(env kit.Envelope) recordCard {
	fields := orderedData(env.Data)
	get := func(keys ...string) string {
		for _, k := range keys {
			for _, kv := range fields {
				if kv.Key == k {
					if s, ok := kv.Val.(string); ok && s != "" {
						return s
					}
				}
			}
		}
		return ""
	}
	u, _ := kit.ParseURI(env.ID)
	title := get("title", "name", "headline", "handle", "username", "text")
	if title == "" {
		title = u.ID()
	}
	return recordCard{
		URI:     env.ID,
		Title:   title,
		Snippet: truncate(get("description", "extract", "summary", "bio", "text"), 160),
		Thumb:   get("thumbnail", "image", "cover", "avatar", "photo"),
		Type:    env.Type,
		Accent:  accent(u.Scheme),
	}
}

// --- links ------------------------------------------------------------------

type linksView struct {
	URI    string
	Accent string
	Groups []linkGroup
}

func (c *Console) linksPage(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("uri")
	u, err := kit.ParseURI(raw)
	if err != nil {
		c.fail(w, r, errs.Usage("%s", err.Error()), raw)
		return
	}
	// Links are derived from the record's envelope, so they share the same fetch
	// job and cache entry as the resource page: viewing one warms the other.
	emit := func(f ant.Fetched) {
		if wantsJSON(r) {
			writeJSON(w, http.StatusOK, flattenLinks(f.Env.Links))
			return
		}
		c.render(w, r, http.StatusOK, "links", "Links of "+u.String(), "",
			linksView{URI: u.String(), Accent: accent(u.Scheme), Groups: linkGroupsOf(f.Env.Links)})
	}
	if f, ok := c.e.Lookup(u); ok {
		emit(f)
		return
	}
	jb := c.jobs.start(fetchKey("get", u.String(), 0, 0, false), func(ctx context.Context) (any, error) {
		return c.e.Dereference(ctx, u, false)
	})
	if wantsJSON(r) {
		ph, v, jerr := jb.wait(r.Context())
		switch ph {
		case jobReady:
			writeJSON(w, http.StatusOK, flattenLinks(v.(ant.Fetched).Env.Links))
		case jobError:
			writeJSONErr(w, jerr)
		default:
			writeJSON(w, http.StatusAccepted, pendingJSON(u))
		}
		return
	}
	ph, v, jerr := c.graced(r, jb)
	switch ph {
	case jobReady:
		emit(v.(ant.Fetched))
	case jobError:
		c.renderError(w, r, jerr, u.String())
	default:
		c.renderLoading(w, r, u, "record", "get", 0, 0, false)
	}
}

// --- resolve ----------------------------------------------------------------

type resolveView struct {
	Input    string
	On       string
	Schemes  []string
	Resolved string
	LiveURL  string
	Err      string
}

func (c *Console) resolve(w http.ResponseWriter, r *http.Request) {
	input := r.URL.Query().Get("input")
	on := r.URL.Query().Get("on")
	if wantsJSON(r) {
		u, err := c.e.Resolve(input, on)
		if err != nil {
			writeJSONErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"uri": u.String()})
		return
	}
	rv := resolveView{Input: input, On: on, Schemes: c.schemes()}
	if input == "" {
		c.render(w, r, http.StatusOK, "resolve", "Resolve", "resolve", rv)
		return
	}
	u, err := c.e.Resolve(input, on)
	if err != nil {
		rv.Err = err.Error()
		c.render(w, r, http.StatusBadRequest, "resolve", "Resolve", "resolve", rv)
		return
	}
	// An unambiguous input (already a URI, or a URL a domain claims) forwards
	// straight to the record; a bare id disambiguated by --on shows the result so
	// the human sees what their id became (8000_ant_serve §11).
	if on == "" {
		http.Redirect(w, r, viewHref(u.String()), http.StatusSeeOther)
		return
	}
	rv.Resolved = u.String()
	rv.LiveURL, _ = c.e.URL(u)
	c.render(w, r, http.StatusOK, "resolve", "Resolved", "resolve", rv)
}

// --- locate (url) -----------------------------------------------------------

type locateView struct {
	URI     string
	LiveURL string
	Err     string
}

func (c *Console) locate(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("uri")
	u, err := kit.ParseURI(raw)
	if err != nil {
		c.fail(w, r, errs.Usage("%s", err.Error()), raw)
		return
	}
	loc, err := c.e.URL(u)
	if wantsJSON(r) {
		if err != nil {
			writeJSONErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": loc})
		return
	}
	lv := locateView{URI: u.String(), LiveURL: loc}
	if err != nil {
		lv.Err = err.Error()
	}
	c.render(w, r, http.StatusOK, "locate", "Live URL", "", lv)
}

// --- graph ------------------------------------------------------------------

type graphView struct {
	URI     string
	Depth   int
	Payload graphPayload
	JSON    string
}

func (c *Console) graph(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("uri")
	u, err := kit.ParseURI(raw)
	if err != nil {
		c.fail(w, r, errs.Usage("%s", err.Error()), raw)
		return
	}
	depth := 1
	if d, e := strconv.Atoi(r.URL.Query().Get("depth")); e == nil && d >= 0 {
		depth = d
	}
	if depth > 3 {
		depth = 3
	}
	dot := r.URL.Query().Get("format") == "dot"
	// A graph walk fans out over the network, so it runs as a background job too.
	// DOT and JSON callers want the bytes and so wait for the job; a browser gets
	// the grace race and a loading screen on a slow walk (8000_ant_serve §24).
	jb := c.jobs.start(fetchKey("graph", u.String(), 0, depth, false), func(ctx context.Context) (any, error) {
		return c.e.Walk(ctx, u, depth)
	})
	if dot || wantsJSON(r) {
		ph, v, jerr := jb.wait(r.Context())
		if ph == jobError {
			c.fail(w, r, jerr, u.String())
			return
		}
		if ph != jobReady {
			writeJSON(w, http.StatusAccepted, pendingJSON(u))
			return
		}
		g := v.(*ant.Graph)
		if dot {
			w.Header().Set("Content-Type", "text/vnd.graphviz; charset=utf-8")
			_, _ = w.Write([]byte(g.Dot()))
			return
		}
		writeJSON(w, http.StatusOK, graphToPayload(u.String(), g))
		return
	}
	ph, v, jerr := c.graced(r, jb)
	switch ph {
	case jobReady:
		payload := graphToPayload(u.String(), v.(*ant.Graph))
		c.render(w, r, http.StatusOK, "graph", "Graph of "+u.String(), "graph",
			graphView{URI: u.String(), Depth: depth, Payload: payload, JSON: prettyJSON(payload)})
	case jobError:
		c.renderError(w, r, jerr, u.String())
	default:
		c.renderLoading(w, r, u, "graph", "graph", 0, depth, false)
	}
}

// --- background fetch plumbing (jobs, loading screen, status poll) ----------

// loadingView is the model for the loading screen shown while a slow fetch runs in
// the background. The poller hits StatusURL and reloads the page the moment the
// work is ready; LiveURL is the manual escape hatch; Crumbs place the pending
// record in the tree (8000_ant_serve §24).
type loadingView struct {
	URI       string
	Scheme    string
	Accent    string
	Kind      string // "record", "collection", "graph" — what is being fetched
	LiveURL   string
	StatusURL string
	Crumbs    []crumb
}

// prefetchCap bounds how many of a record's links are warmed on view, so a page
// with hundreds of links does not fan out without limit.
const prefetchCap = 6

// fetchKey is the dedup/identity key for a background fetch, shared by the page
// handlers and the status endpoint so a poll finds the very job its page started.
// view and links collapse onto one "get" record fetch; ls and graph key on their
// own parameters.
func fetchKey(op, uri string, n, depth int, refresh bool) string {
	switch op {
	case "ls":
		return "ls:" + uri + ":" + strconv.Itoa(n)
	case "graph":
		return "graph:" + uri + ":" + strconv.Itoa(depth)
	default: // "get"
		if refresh {
			return "get:" + uri + ":fresh"
		}
		return "get:" + uri
	}
}

// pendingJSON is the body a JSON caller gets when a fetch is still running past
// the client's own deadline (rare; the script usually waits for the data).
func pendingJSON(u kit.URI) map[string]string {
	return map[string]string{"status": "pending", "uri": u.String()}
}

// renderLoading shows the loading screen for a page whose fetch is still running.
// The screen polls the status endpoint and reloads itself the instant the work is
// ready (or failed), so the eventual render is the normal page, not a spinner.
func (c *Console) renderLoading(w http.ResponseWriter, r *http.Request, u kit.URI, kind, op string, n, depth int, refresh bool) {
	liveURL, _ := c.e.URL(u)
	lv := loadingView{
		URI:       u.String(),
		Scheme:    u.Scheme,
		Accent:    accent(u.Scheme),
		Kind:      kind,
		LiveURL:   liveURL,
		StatusURL: statusURL(op, u.String(), n, depth, refresh),
		Crumbs:    crumbsFor(u),
	}
	c.render(w, r, http.StatusAccepted, "loading", "Fetching "+u.String(), "loading", lv)
}

// status reports the phase of a background fetch as JSON, so the loading page's
// poller can reload the instant the work is ready or failed. It always answers
// JSON and never starts a fetch — it only peeks at one a page already launched.
func (c *Console) status(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n, _ := strconv.Atoi(q.Get("n"))
	depth, _ := strconv.Atoi(q.Get("depth"))
	var key string
	if q.Get("op") == "search" {
		key = searchKey(q.Get("on"), q.Get("q"), n)
	} else {
		key = fetchKey(q.Get("op"), q.Get("uri"), n, depth, q.Get("refresh") == "1")
	}
	jb, ok := c.jobs.peek(key)
	if !ok {
		// No job: it finished and was swept, or this is a stale poll. Either way, tell
		// the poller to reload — the page then hits the cache or re-runs the fetch.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		return
	}
	switch ph, _, jerr := jb.snapshot(); ph {
	case jobReady:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	case jobError:
		writeJSON(w, http.StatusOK, map[string]string{"status": "error", "error": jerr.Error()})
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
	}
}

// prefetchLinks warms the cache for the records this one links to, so the next
// click opens instantly. It is fire-and-forget and bounded: only not-yet-cached
// targets, only up to prefetchCap, each run through the prefetch limiter so a site
// is never stampeded (8000_ant_serve §24).
func (c *Console) prefetchLinks(links map[string][]string) {
	n := 0
	for _, targets := range links {
		for _, t := range targets {
			if n >= prefetchCap {
				return
			}
			tu, err := kit.ParseURI(t)
			if err != nil || c.e.Cached(tu) {
				continue
			}
			n++
			target := tu
			c.jobs.prefetch(fetchKey("get", target.String(), 0, 0, false), func(ctx context.Context) (any, error) {
				return c.e.Dereference(ctx, target, false)
			})
		}
	}
}

// flattenLinks returns a record's link targets as a sorted, de-duplicated flat
// list, the shape the links JSON endpoint has always returned (a [] for none).
func flattenLinks(m map[string][]string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, targets := range m {
		for _, t := range targets {
			if !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
	}
	sort.Strings(out)
	return out
}

// --- browse (the data tree as directories) ----------------------------------

// browseLeafCap bounds how many record cards a single authority folder renders,
// so a large cache cannot make one page unbounded. Truncation is reported.
const browseLeafCap = 300

type browseView struct {
	Root       string
	Prefix     string // canonical prefix string for this node ("" at root)
	Crumbs     []crumb
	Scheme     string // the scheme in context, "" at root
	Accent     string
	Folders    []browseFolder
	Records    []recordCard
	Searchable bool
	Examples   []string // try-me URIs offered when a folder has no cache yet
	Total      int      // records cached under this node
	Shown      int      // records rendered as cards (<= Total, capped)
}

type browseFolder struct {
	Name   string
	Href   string
	Count  int
	Accent string
}

// browse renders the on-disk data tree as a directory listing: the root lists
// every registered domain as a folder, a domain lists the record types it has
// cached, and a type lists its records as cards that open the cached view. It is
// pure filesystem work over LL, so it never touches the network; the search box
// and example URIs are the bridges out to a live fetch (8000_ant_serve §22).
func (c *Console) browse(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	segs := splitPrefix(prefix)
	canon := joinPrefix(segs)
	depth := len(segs)

	// Root: list the registered domains as folders, scoped to ant's own data and
	// never the whole shared root. Walking $HOME/data wholesale is both slow (it
	// holds many other tools' trees) and wrong (it would surface their files as ant
	// records). Each folder's count and the JSON listing come from per-domain
	// listings, which the in-memory index keeps cheap.
	if depth == 0 {
		bv := browseView{Root: c.e.Root(), Prefix: "", Crumbs: crumbsForPrefix(segs)}
		var all []string
		for _, d := range c.e.Domains() {
			cu, e := c.e.LL(d.Scheme + "://")
			if e == nil {
				all = append(all, cu...)
			}
			bv.Total += len(cu)
			bv.Folders = append(bv.Folders, browseFolder{
				Name:   d.Scheme,
				Href:   browseHref(d.Scheme + "://"),
				Count:  len(cu),
				Accent: accent(d.Scheme),
			})
		}
		if wantsJSON(r) {
			sort.Strings(all)
			writeJSON(w, http.StatusOK, all)
			return
		}
		c.render(w, r, http.StatusOK, "browse", "Browse the data tree", "browse", bv)
		return
	}

	uris, err := c.e.LL(canon)
	if err != nil {
		c.fail(w, r, err, prefix)
		return
	}
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, uris)
		return
	}

	bv := browseView{Root: c.e.Root(), Prefix: canon, Crumbs: crumbsForPrefix(segs), Total: len(uris)}
	bv.Scheme = segs[0]
	bv.Accent = accent(segs[0])
	bv.Searchable = c.e.Searchable(segs[0])
	bv.Examples = exampleURIs(segs[0])

	// Deeper: group the cached URIs by their segment at this depth. A child with
	// more segments below it is a folder; one that terminates here is a record.
	folderCount := map[string]int{}
	var folderOrder []string
	var leaves []string
	for _, uri := range uris {
		us := uriSegs(uri)
		if len(us) <= depth || !segsHasPrefix(us, segs) {
			continue
		}
		child := us[depth]
		if len(us) == depth+1 {
			leaves = append(leaves, uri)
			continue
		}
		if _, seen := folderCount[child]; !seen {
			folderOrder = append(folderOrder, child)
		}
		folderCount[child]++
	}
	for _, name := range folderOrder {
		child := joinPrefix(append(append([]string{}, segs...), name))
		bv.Folders = append(bv.Folders, browseFolder{
			Name:   name,
			Href:   browseHref(child),
			Count:  folderCount[name],
			Accent: accent(segs[0]),
		})
	}
	for i, uri := range leaves {
		if i >= browseLeafCap {
			break
		}
		bv.Records = append(bv.Records, c.cardFromCache(uri))
	}
	bv.Shown = len(bv.Records)
	c.render(w, r, http.StatusOK, "browse", "Browse "+canon, "browse", bv)
}

// cardFromCache builds a list card for a record already on disk. It reads the
// cache only (the URI came from LL, so the fetch never reaches the network), and
// falls back to a bare-URI card if the file is unreadable.
func (c *Console) cardFromCache(uri string) recordCard {
	u, err := kit.ParseURI(uri)
	if err != nil {
		return recordCard{URI: uri, Title: uri}
	}
	f, err := c.e.Dereference(context.Background(), u, false)
	if err != nil {
		return recordCard{URI: uri, Title: u.ID(), Accent: accent(u.Scheme)}
	}
	return cardFromEnv(f.Env)
}

// --- search -----------------------------------------------------------------

type searchView struct {
	Scheme   string
	Accent   string
	Query    string
	N        int
	Schemes  []string // searchable schemes, for the selector
	Cards    []recordCard
	Searched bool
	Err      string
}

// search runs a domain's free-text search and renders the hits as cards that open
// each result's record (cache-first). The box is shown for every domain that
// registered a search op; a domain without one is reported, not hidden, so the UI
// stays honest (8000_ant_serve §22.1).
func (c *Console) search(w http.ResponseWriter, r *http.Request) {
	scheme := r.URL.Query().Get("on")
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))

	if wantsJSON(r) {
		if scheme == "" || query == "" {
			writeJSONErr(w, errs.Usage("search needs ?on=<scheme>&q=<query>"))
			return
		}
		jb := c.jobs.start(searchKey(scheme, query, n), c.searchRun(scheme, query, n))
		ph, v, jerr := jb.wait(r.Context())
		switch ph {
		case jobReady:
			writeJSON(w, http.StatusOK, v.([]kit.Envelope))
		case jobError:
			writeJSONErr(w, jerr)
		default:
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "pending"})
		}
		return
	}

	sv := searchView{Scheme: scheme, Accent: accent(scheme), Query: query, N: n, Schemes: c.searchSchemes()}
	if scheme == "" || query == "" {
		c.render(w, r, http.StatusOK, "search", "Search", "search", sv)
		return
	}
	if !c.e.Searchable(scheme) {
		sv.Err = scheme + " does not support search"
		c.render(w, r, http.StatusBadRequest, "search", "Search", "search", sv)
		return
	}
	// A search is always live (results are previews, never cached), so it too runs
	// as a background job: a quick query renders inline, a slow one shows the
	// loading screen, and neither path can time out (8000_ant_serve §24).
	jb := c.jobs.start(searchKey(scheme, query, n), c.searchRun(scheme, query, n))
	ph, v, jerr := c.graced(r, jb)
	switch ph {
	case jobReady:
		sv.Searched = true
		for _, env := range v.([]kit.Envelope) {
			sv.Cards = append(sv.Cards, cardFromEnv(env))
		}
		c.render(w, r, http.StatusOK, "search", "Search: "+query, "search", sv)
	case jobError:
		sv.Err = jerr.Error()
		c.render(w, r, statusFor(jerr), "search", "Search", "search", sv)
	default:
		c.renderSearchLoading(w, r, scheme, query, n)
	}
}

// searchRun is the search job's work: a context-aware free-text query.
func (c *Console) searchRun(scheme, query string, n int) runFn {
	return func(ctx context.Context) (any, error) {
		return c.e.Search(ctx, scheme, query, n)
	}
}

// searchKey is the dedup key for a search job, distinct from a record fetch so a
// query and a URI never collide.
func searchKey(scheme, query string, n int) string {
	return "search:" + scheme + ":" + strconv.Itoa(n) + ":" + query
}

// renderSearchLoading shows the loading screen for a slow query. Its reload target
// is the search URL itself, so when the job is ready the reload renders the hits.
func (c *Console) renderSearchLoading(w http.ResponseWriter, r *http.Request, scheme, query string, n int) {
	c.render(w, r, http.StatusAccepted, "loading", "Searching "+scheme, "loading", loadingView{
		URI:       query,
		Scheme:    scheme,
		Accent:    accent(scheme),
		Kind:      "search results",
		StatusURL: searchStatusURL(scheme, query, n),
	})
}

// searchSchemes is the set of registered schemes that support search, for the
// search form's domain selector.
func (c *Console) searchSchemes() []string {
	var out []string
	for _, d := range c.e.Domains() {
		if c.e.Searchable(d.Scheme) {
			out = append(out, d.Scheme)
		}
	}
	return out
}

// --- domain -----------------------------------------------------------------

type domainView struct {
	Info     ant.DomainInfo
	Accent   string
	Examples []string
}

func (c *Console) domainPage(w http.ResponseWriter, r *http.Request) {
	scheme := r.URL.Query().Get("scheme")
	info, ok := c.e.Domain(scheme)
	if !ok {
		c.fail(w, r, errs.NotFound("no domain %q", scheme), "")
		return
	}
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, info)
		return
	}
	c.render(w, r, http.StatusOK, "domain", info.Scheme, "",
		domainView{Info: info, Accent: accent(info.Scheme), Examples: exampleURIs(info.Scheme)})
}

// --- about ------------------------------------------------------------------

type aboutView struct {
	Version string
	Commit  string
	Date    string
	Domains []ant.DomainInfo
}

func (c *Console) about(w http.ResponseWriter, r *http.Request) {
	av := aboutView{Version: c.build.Version, Commit: c.build.Commit, Date: c.build.Date, Domains: c.e.Domains()}
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, av)
		return
	}
	c.render(w, r, http.StatusOK, "about", "About ant", "about", av)
}

// --- export (the one POST) --------------------------------------------------

func (c *Console) export(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		c.fail(w, r, errs.Usage("export requires POST"), "")
		return
	}
	if !sameOrigin(r) {
		c.fail(w, r, errs.Usage("cross-origin export refused"), "")
		return
	}
	_ = r.ParseForm()
	raw := r.FormValue("uri")
	u, err := kit.ParseURI(raw)
	if err != nil {
		c.fail(w, r, errs.Usage("%s", err.Error()), raw)
		return
	}
	follow, _ := strconv.Atoi(r.FormValue("follow"))
	md := r.FormValue("md") == "on" || r.FormValue("md") == "true"
	ctx, cancel := reqCtx(r)
	defer cancel()
	rep, err := c.e.Export(ctx, u, follow, md)
	if err != nil {
		c.fail(w, r, err, u.String())
		return
	}
	if wantsJSON(r) {
		writeJSON(w, http.StatusOK, rep)
		return
	}
	http.Redirect(w, r, viewHref(u.String())+"&exported=1", http.StatusSeeOther)
}

// --- not found --------------------------------------------------------------

func (c *Console) notFound(w http.ResponseWriter, r *http.Request) {
	if wantsJSON(r) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	c.render(w, r, http.StatusNotFound, "notfound", "Not found", "", nil)
}

// fail renders an error as JSON or the styled error page per negotiation.
func (c *Console) fail(w http.ResponseWriter, r *http.Request, err error, uri string) {
	if wantsJSON(r) {
		writeJSONErr(w, err)
		return
	}
	c.renderError(w, r, err, uri)
}

// schemes lists the canonical schemes for the resolve form's <select>.
func (c *Console) schemes() []string {
	var out []string
	for _, d := range c.e.Domains() {
		out = append(out, d.Scheme)
	}
	return out
}

// sameOrigin is a defense-in-depth check on the one state-changing POST.
func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true // non-browser client (curl) or same-origin form without Origin
	}
	return o == "http://"+r.Host || o == "https://"+r.Host
}
