package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// findMsg returns the first message of type T in msgs.
func findMsg[T any](msgs []tea.Msg) (T, bool) {
	for _, m := range msgs {
		if t, ok := m.(T); ok {
			return t, true
		}
	}
	var zero T
	return zero, false
}

func TestDashboardListsAndOpens(t *testing.T) {
	var s Screen = newDashboardScreen(testDeps(fakeDeref{}))
	s = sized(s, 80, 24)
	if out := s.View(80, 24); !strings.Contains(out, "demo") {
		t.Fatalf("dashboard should list the demo domain:\n%s", out)
	}

	_, cmd := s.Update(kEnter)
	if pm, ok := findMsg[pushMsg](drain(cmd)); !ok {
		t.Fatal("enter on a domain should push a screen")
	} else if _, ok := pm.Screen.(*domainScreen); !ok {
		t.Fatalf("enter should push a domain screen, got %T", pm.Screen)
	}

	_, cmd = s.Update(kRune('b'))
	if pm, ok := findMsg[pushMsg](drain(cmd)); !ok {
		t.Fatal("b should push the browse screen")
	} else if _, ok := pm.Screen.(*browseScreen); !ok {
		t.Fatalf("b should push a browse screen, got %T", pm.Screen)
	}
}

func TestResourceWarmPaintsWithoutLoading(t *testing.T) {
	s := newResourceScreen(testDeps(fakeDeref{}), mustURI("demo://widget/42"), false)
	_ = sized(s, 100, 30)
	cmd := s.Init()
	if s.Loading() {
		t.Fatal("a warm record (cache hit) should not be loading after Init")
	}
	if !s.Cached() {
		t.Fatal("a warm record should report Cached")
	}
	if cmd != nil {
		t.Fatal("warm, non-refresh Init should issue no command")
	}
	out := s.View(100, 30)
	if !strings.Contains(out, "demo://widget/42") {
		t.Fatalf("resource view should show the URI:\n%s", out)
	}
}

func TestResourceColdDerefsThenInstalls(t *testing.T) {
	s := newResourceScreen(testDeps(fakeDeref{cold: true}), mustURI("demo://widget/42"), false)
	_ = sized(s, 100, 30)
	cmd := s.Init()
	if !s.Loading() {
		t.Fatal("a cold record should be loading after Init")
	}
	fm, ok := findMsg[fetchedMsg](drain(cmd))
	if !ok {
		t.Fatal("cold Init should issue a deref command")
	}
	ns, _ := s.Update(fm)
	if ns.Loading() {
		t.Fatal("resource should stop loading once the fetch lands")
	}
	if out := ns.View(100, 30); !strings.Contains(out, "demo/widget") {
		t.Fatalf("installed resource should render its type:\n%s", out)
	}
}

func TestResourceModeCycle(t *testing.T) {
	s := newResourceScreen(testDeps(fakeDeref{}), mustURI("demo://widget/42"), false)
	_ = sized(s, 100, 30)
	s.Init()
	if s.mode != modeData {
		t.Fatalf("resource should open in data mode, got %v", s.mode)
	}
	s.Update(kRune('v')) // data -> body (HasBody)
	if s.mode != modeBody {
		t.Fatalf("v should cycle to body, got %v", s.mode)
	}
	s.Update(kRune('v')) // body -> raw
	if s.mode != modeRaw {
		t.Fatalf("v should cycle to raw, got %v", s.mode)
	}
	s.Update(kRune('v')) // raw -> data
	if s.mode != modeData {
		t.Fatalf("v should cycle back to data, got %v", s.mode)
	}
}

func TestResourceCopyToast(t *testing.T) {
	s := newResourceScreen(testDeps(fakeDeref{}), mustURI("demo://widget/42"), false)
	_ = sized(s, 100, 30)
	s.Init()
	_, cmd := s.Update(kRune('y'))
	tm, ok := findMsg[toastMsg](drain(cmd))
	if !ok {
		t.Fatal("y should flash a copy toast")
	}
	if !strings.Contains(tm.Text, "demo://widget/42") {
		t.Fatalf("copy toast should name the URI, got %q", tm.Text)
	}
}

func TestResourceGraphAndLinksKeys(t *testing.T) {
	s := newResourceScreen(testDeps(fakeDeref{}), mustURI("demo://widget/42"), false)
	_ = sized(s, 100, 30)
	s.Init()

	_, cmd := s.Update(kRune('x'))
	if pm, ok := findMsg[pushMsg](drain(cmd)); !ok {
		t.Fatal("x should push a screen")
	} else if _, ok := pm.Screen.(*graphScreen); !ok {
		t.Fatalf("x should push the graph screen, got %T", pm.Screen)
	}

	_, cmd = s.Update(kRune('L'))
	if pm, ok := findMsg[pushMsg](drain(cmd)); !ok {
		t.Fatal("L should push a screen")
	} else if _, ok := pm.Screen.(*linksScreen); !ok {
		t.Fatalf("L should push the links screen, got %T", pm.Screen)
	}
}

func TestResourceFollowLink(t *testing.T) {
	s := newResourceScreen(testDeps(fakeDeref{}), mustURI("demo://widget/42"), false)
	_ = sized(s, 100, 30)
	s.Init()
	s.Update(kTab) // focus the links pane
	if s.focus != focusLinks {
		t.Fatal("tab should move focus to the links pane")
	}
	_, cmd := s.Update(kEnter)
	nm, ok := findMsg[navigateMsg](drain(cmd))
	if !ok {
		t.Fatal("enter on a focused link should navigate")
	}
	if nm.URI.String() != "demo://maker/m1" {
		t.Fatalf("should follow the maker link, got %s", nm.URI)
	}
}

func TestCollectionListsAndFollows(t *testing.T) {
	var s Screen = newCollectionScreen(testDeps(fakeDeref{}), mustURI("demo://maker/m1"))
	s = sized(s, 80, 24)
	lm, ok := findMsg[listedMsg](drain(s.Init()))
	if !ok {
		t.Fatal("collection Init should issue a list command")
	}
	s, _ = s.Update(lm)
	if out := s.View(80, 24); !strings.Contains(out, "widget/1") {
		t.Fatalf("collection should list members:\n%s", out)
	}
	_, cmd := s.Update(kEnter)
	if _, ok := findMsg[navigateMsg](drain(cmd)); !ok {
		t.Fatal("enter on a member should navigate")
	}
}

func TestSearchEditRunFollow(t *testing.T) {
	d := testDeps(fakeDeref{})
	s := newSearchScreen(d, "demo")
	_ = sized(s, 80, 24)
	s.Init()
	if !s.Capturing() {
		t.Fatal("search should capture keys while the query field is focused")
	}
	s.Update(kRune('h'))
	s.Update(kRune('i'))
	_, cmd := s.Update(kEnter)
	sm, ok := findMsg[searchedMsg](drain(cmd))
	if !ok {
		t.Fatal("enter in the query field should run a search")
	}
	if s.Capturing() {
		t.Fatal("running a query should release the capture")
	}
	s.Update(sm)
	_, cmd = s.Update(kEnter)
	if _, ok := findMsg[navigateMsg](drain(cmd)); !ok {
		t.Fatal("enter on a result should navigate")
	}
}

func TestSearchSeededQueryRunsOnInit(t *testing.T) {
	s := newSearchScreenWith(testDeps(fakeDeref{}), "demo", "rockets")
	_ = sized(s, 80, 24)
	sm, ok := findMsg[searchedMsg](drain(s.Init()))
	if !ok {
		t.Fatal("a seeded search should run on Init")
	}
	if sm.Query != "rockets" {
		t.Fatalf("seeded query should be carried through, got %q", sm.Query)
	}
	if s.Capturing() {
		t.Fatal("a seeded search should open on results, not the field")
	}
}

func TestGraphTreeAndDotToggle(t *testing.T) {
	s := newGraphScreen(testDeps(fakeDeref{}), mustURI("demo://widget/42"), 1)
	_ = sized(s, 80, 24)
	wm, ok := findMsg[walkedMsg](drain(s.Init()))
	if !ok {
		t.Fatal("graph Init should issue a walk command")
	}
	s.Update(wm)
	if len(s.pick.items) != 2 {
		t.Fatalf("graph should list both nodes, got %d", len(s.pick.items))
	}
	if out := s.View(80, 24); !strings.Contains(out, "tree") {
		t.Fatalf("graph should open in tree view:\n%s", out)
	}
	s.Update(kRune('v'))
	if !s.dotMode {
		t.Fatal("v should toggle the dot view")
	}
	if out := s.View(80, 24); !strings.Contains(out, "digraph") {
		t.Fatalf("dot view should render the DOT:\n%s", out)
	}
}

func TestGraphEnterFollowsNode(t *testing.T) {
	s := newGraphScreen(testDeps(fakeDeref{}), mustURI("demo://widget/42"), 1)
	_ = sized(s, 80, 24)
	wm, _ := findMsg[walkedMsg](drain(s.Init()))
	s.Update(wm)
	s.Update(kDown) // move to the second node
	_, cmd := s.Update(kEnter)
	if _, ok := findMsg[navigateMsg](drain(cmd)); !ok {
		t.Fatal("enter on a node should navigate to it")
	}
}

func TestBrowseRootFoldersWithCounts(t *testing.T) {
	var s Screen = newBrowseScreen(testDeps(fakeDeref{}), "")
	s = sized(s, 80, 24)
	msgs := drain(s.Init())
	lm, ok := findMsg[llMsg](msgs)
	if !ok {
		t.Fatal("browse root should probe each domain")
	}
	s, _ = s.Update(lm)
	out := s.View(80, 24)
	if !strings.Contains(out, "demo") || !strings.Contains(out, "cached") {
		t.Fatalf("browse root should show domains with cache counts:\n%s", out)
	}
	_, cmd := s.Update(kEnter)
	if pm, ok := findMsg[pushMsg](drain(cmd)); !ok {
		t.Fatal("enter on a domain folder should descend")
	} else if bs, ok := pm.Screen.(*browseScreen); !ok || bs.prefix != "demo://" {
		t.Fatalf("descending should push browse of demo://, got %T %q", pm.Screen, prefixOf(pm.Screen))
	}
}

func TestBrowsePrefixGroupsRecords(t *testing.T) {
	var s Screen = newBrowseScreen(testDeps(fakeDeref{}), "demo://maker")
	s = sized(s, 80, 24)
	lm, ok := findMsg[llMsg](drain(s.Init()))
	if !ok {
		t.Fatal("a browse prefix should list its cached URIs")
	}
	s, _ = s.Update(lm)
	if out := s.View(80, 24); !strings.Contains(out, "maker/m1") {
		t.Fatalf("browse should surface the cached record as a leaf:\n%s", out)
	}
	_, cmd := s.Update(kEnter)
	if _, ok := findMsg[navigateMsg](drain(cmd)); !ok {
		t.Fatal("enter on a record leaf should navigate")
	}
}

func prefixOf(s Screen) string {
	if b, ok := s.(*browseScreen); ok {
		return b.prefix
	}
	return ""
}

func TestDomainOpensSearchAndBrowse(t *testing.T) {
	var s Screen = newDomainScreen(testDeps(fakeDeref{}), "demo")
	s = sized(s, 80, 24)
	_, cmd := s.Update(kRune('/'))
	if pm, ok := findMsg[pushMsg](drain(cmd)); !ok {
		t.Fatal("/ on a searchable domain should push search")
	} else if _, ok := pm.Screen.(*searchScreen); !ok {
		t.Fatalf("/ should push the search screen, got %T", pm.Screen)
	}
	_, cmd = s.Update(kRune('b'))
	if pm, ok := findMsg[pushMsg](drain(cmd)); !ok {
		t.Fatal("b should push browse")
	} else if bs, ok := pm.Screen.(*browseScreen); !ok || bs.prefix != "demo://" {
		t.Fatalf("b should browse demo://, got %T", pm.Screen)
	}
}
