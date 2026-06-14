package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestOmniboxVerbs(t *testing.T) {
	d := testDeps(fakeDeref{})

	cases := []struct {
		input string
		check func(t *testing.T, s Screen)
	}{
		{"browse demo://", func(t *testing.T, s Screen) {
			b, ok := s.(*browseScreen)
			if !ok || b.prefix != "demo://" {
				t.Fatalf("browse verb should open browse demo://, got %T", s)
			}
		}},
		{"domain demo", func(t *testing.T, s Screen) {
			if _, ok := s.(*domainScreen); !ok {
				t.Fatalf("domain verb should open a domain screen, got %T", s)
			}
		}},
		{"search demo rockets", func(t *testing.T, s Screen) {
			ss, ok := s.(*searchScreen)
			if !ok || ss.pending != "rockets" {
				t.Fatalf("search verb should seed the query, got %T", s)
			}
		}},
		{"graph demo://widget/42 2", func(t *testing.T, s Screen) {
			g, ok := s.(*graphScreen)
			if !ok || g.depth != 2 {
				t.Fatalf("graph verb should carry the depth, got %T", s)
			}
		}},
		{"ls demo://maker/m1", func(t *testing.T, s Screen) {
			if _, ok := s.(*collectionScreen); !ok {
				t.Fatalf("ls verb should open a collection, got %T", s)
			}
		}},
	}
	for _, tc := range cases {
		o := newOmnibox(d)
		o.open(tc.input)
		_, cmd := o.update(kEnter)
		pm, ok := findMsg[pushMsg](drain(cmd))
		if !ok {
			t.Fatalf("%q should push a screen", tc.input)
		}
		tc.check(t, pm.Screen)
	}
}

func TestOmniboxBareInputResolves(t *testing.T) {
	o := newOmnibox(testDeps(fakeDeref{}))
	o.open("demo://widget/42")
	_, cmd := o.update(kEnter)
	if _, ok := findMsg[resolvedMsg](drain(cmd)); !ok {
		t.Fatal("a bare address should run through Resolve")
	}
}

func TestAppBackForwardStack(t *testing.T) {
	a := newApp(testDeps(fakeDeref{}), nil)
	m, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a = m.(*App)
	if len(a.stack) != 1 {
		t.Fatalf("app should start on the dashboard alone, got %d", len(a.stack))
	}

	m, _ = a.Update(navigateMsg{URI: mustURI("demo://widget/42")})
	a = m.(*App)
	if len(a.stack) != 2 {
		t.Fatalf("navigate should push a resource, got depth %d", len(a.stack))
	}
	if _, ok := a.active().(*resourceScreen); !ok {
		t.Fatalf("top of stack should be the resource, got %T", a.active())
	}

	m, _ = a.Update(backMsg{})
	a = m.(*App)
	if len(a.stack) != 1 {
		t.Fatalf("back should pop to the dashboard, got %d", len(a.stack))
	}

	m, _ = a.Update(forwardMsg{})
	a = m.(*App)
	if len(a.stack) != 2 {
		t.Fatalf("forward should re-push, got %d", len(a.stack))
	}
}

func TestAppGlobalKeys(t *testing.T) {
	a := newApp(testDeps(fakeDeref{}), nil)
	m, _ := a.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	a = m.(*App)

	// Theme toggle flips the palette.
	wasDark := a.d.sty.dark
	m, _ = a.Update(kRune('T'))
	a = m.(*App)
	if a.d.sty.dark == wasDark {
		t.Fatal("T should toggle the theme")
	}

	// ':' opens the omnibox.
	m, _ = a.Update(kRune(':'))
	a = m.(*App)
	if !a.omni.active {
		t.Fatal(": should open the omnibox")
	}
	// While the omnibox is open, Esc closes it rather than going back.
	m, _ = a.Update(kEsc)
	a = m.(*App)
	if a.omni.active {
		t.Fatal("esc should close the omnibox")
	}

	// '?' opens help; any key closes it.
	m, _ = a.Update(kRune('?'))
	a = m.(*App)
	if !a.helpOpen {
		t.Fatal("? should open help")
	}

	// 'q' quits.
	_, cmd := a.Update(kRune('q'))
	if _, ok := findMsg[tea.QuitMsg](drain(cmd)); !ok {
		// help is open, so the first key closes help instead of quitting; reissue.
		a.helpOpen = false
		_, cmd = a.Update(kRune('q'))
		if _, ok := findMsg[tea.QuitMsg](drain(cmd)); !ok {
			t.Fatal("q should quit")
		}
	}
}
