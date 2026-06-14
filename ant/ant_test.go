package ant_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// A fake domain exercises the whole data surface offline: the records come from
// the handlers, not the network, so export/ll/import/graph are tested without a
// live site. It registers under a throwaway scheme on init.

type book struct {
	ID       string   `json:"id" kit:"id"`
	Title    string   `json:"title"`
	Blurb    string   `json:"blurb" kit:"body"`
	AuthorID string   `json:"author_id" kit:"link,kind=fake/author"`
	Similar  []string `json:"similar" kit:"link,kind=fake/book,optional"`
}

type author struct {
	ID   string `json:"id" kit:"id"`
	Name string `json:"name"`
}

type idArg struct {
	ID string `kit:"arg"`
}

type fakeDomain struct{}

func (fakeDomain) Info() kit.DomainInfo {
	return kit.DomainInfo{Scheme: "fake", Hosts: []string{"fake.example"}, Identity: kit.Identity{Binary: "fake"}}
}

func (fakeDomain) Register(app *kit.App) {
	kit.Handle(app, kit.OpMeta{Name: "book", URIType: "book", Single: true, Resolver: true,
		Args: []kit.Arg{{Name: "ref"}}},
		func(_ context.Context, in idArg, emit func(book) error) error {
			return emit(book{ID: in.ID, Title: "Book " + in.ID, Blurb: "A story about " + in.ID,
				AuthorID: "a1", Similar: []string{"b2"}})
		})
	kit.Handle(app, kit.OpMeta{Name: "author", URIType: "author", Single: true, Resolver: true,
		Args: []kit.Arg{{Name: "ref"}}},
		func(_ context.Context, in idArg, emit func(author) error) error {
			return emit(author{ID: in.ID, Name: "Author " + in.ID})
		})
}

func (fakeDomain) Classify(input string) (string, string, error) { return "book", input, nil }
func (fakeDomain) Locate(typ, id string) (string, error) {
	return "https://fake.example/" + typ + "/" + id, nil
}

func init() { kit.Register(fakeDomain{}) }

func newEngine(t *testing.T) (*ant.Engine, string) {
	t.Helper()
	root := t.TempDir()
	clock := func() time.Time { return time.Unix(1700000000, 0) }
	e, err := ant.New(ant.WithRoot(root), ant.WithClock(clock))
	if err != nil {
		t.Fatal(err)
	}
	return e, root
}

func TestGetAndBody(t *testing.T) {
	e, _ := newEngine(t)
	u, _ := kit.ParseURI("fake://book/b1")
	env, err := e.Get(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if env.ID != "fake://book/b1" || env.Type != "fake/book" {
		t.Errorf("envelope id/type = %q/%q", env.ID, env.Type)
	}
	if env.Fetched == "" {
		t.Error("missing @fetched")
	}
	if got := env.Links["author_id"]; len(got) != 1 || got[0] != "fake://author/a1" {
		t.Errorf("links author = %v", env.Links)
	}
	if body, ok := e.BodyOf(env); !ok || body != "A story about b1" {
		t.Errorf("body = %q/%v", body, ok)
	}
}

func TestExportFollowThenLLAndImport(t *testing.T) {
	e, root := newEngine(t)
	u, _ := kit.ParseURI("fake://book/b1")

	rep, err := e.Export(context.Background(), u, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Errors) != 0 {
		t.Fatalf("export errors: %v", rep.Errors)
	}
	// b1 (json+md), its author a1 (json), and similar b2 (json+md).
	want := filepath.Join(root, "fake", "book", "b1.json")
	found := false
	for _, p := range rep.Written {
		if p == want {
			found = true
		}
	}
	if !found {
		t.Errorf("b1.json not in written set %v", rep.Written)
	}

	uris, err := e.LL("fake://")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, u := range uris {
		got[u] = true
	}
	for _, w := range []string{"fake://book/b1", "fake://book/b2", "fake://author/a1"} {
		if !got[w] {
			t.Errorf("ll missing %q (got %v)", w, uris)
		}
	}

	env, err := e.Import(want)
	if err != nil {
		t.Fatal(err)
	}
	if env["@id"] != "fake://book/b1" {
		t.Errorf("import @id = %v", env["@id"])
	}
}

func TestWalkGraph(t *testing.T) {
	e, _ := newEngine(t)
	u, _ := kit.ParseURI("fake://book/b1")
	g, err := e.Walk(context.Background(), u, 1)
	if err != nil {
		t.Fatal(err)
	}
	// b1 -> author a1 and b1 -> similar b2 are the two edges from the root.
	edges := map[string]bool{}
	for _, ed := range g.Edges {
		edges[ed.From+" "+ed.To] = true
	}
	if !edges["fake://book/b1 fake://author/a1"] {
		t.Errorf("missing author edge: %v", g.Edges)
	}
	if !edges["fake://book/b1 fake://book/b2"] {
		t.Errorf("missing similar edge: %v", g.Edges)
	}
	if dot := g.Dot(); len(dot) == 0 || dot[:7] != "digraph" {
		t.Errorf("dot output malformed: %q", dot)
	}
}
