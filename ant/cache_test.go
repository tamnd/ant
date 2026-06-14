package ant_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// TestLLIndexIsCachedAndWriteThrough proves the in-memory listing index: a repeat
// LL is served from memory (so a file written behind the Engine's back is not
// seen), while a record written through the Engine appears at once (write-through
// keeps the index warm without a re-walk). This is what keeps the web console's
// dashboard and browse pages fast as the data tree grows.
func TestLLIndexIsCachedAndWriteThrough(t *testing.T) {
	e, root := newEngine(t)
	ctx := context.Background()

	// Seed the cache entry for the prefix with an initial walk (empty tree).
	if got, err := e.LL("fake://"); err != nil || len(got) != 0 {
		t.Fatalf("initial LL = %v, %v; want empty", got, err)
	}

	// Export a record through the Engine: write-through must fold it into the
	// already-cached listing.
	u, _ := kit.ParseURI("fake://book/b1")
	if _, err := e.Export(ctx, u, 0, false); err != nil {
		t.Fatal(err)
	}
	if got, err := e.LL("fake://"); err != nil || !has(got, "fake://book/b1") {
		t.Fatalf("after Export, LL = %v, %v; want it to contain b1", got, err)
	}

	// Write a record file directly to disk, behind the Engine's back. The cache is
	// authoritative for the session, so LL must NOT pick it up.
	stray := filepath.Join(root, "fake", "book", "stray.json")
	if err := os.WriteFile(stray, []byte(`{"@id":"fake://book/stray"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.LL("fake://"); has(got, "fake://book/stray") {
		t.Errorf("LL saw a file written behind the Engine's back: %v", got)
	}

	// A fresh Engine on the same root walks from scratch and does see the stray,
	// proving the file was really on disk and the cache (not a missing write) hid it.
	e2, err := ant.New(ant.WithRoot(root))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := e2.LL("fake://"); !has(got, "fake://book/stray") {
		t.Errorf("fresh Engine missed the on-disk stray: %v", got)
	}
}

// TestDereferenceWriteBackIndexes proves a cache-first Dereference miss writes the
// record back and folds it into the listing index, so it shows up in browse with
// no re-walk.
func TestDereferenceWriteBackIndexes(t *testing.T) {
	e, _ := newEngine(t)
	ctx := context.Background()

	if got, _ := e.LL("fake://"); len(got) != 0 {
		t.Fatalf("expected empty start, got %v", got)
	}
	u, _ := kit.ParseURI("fake://book/b9")
	if _, err := e.Dereference(ctx, u, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.LL("fake://"); !has(got, "fake://book/b9") {
		t.Errorf("Dereference did not index the written record: %v", got)
	}
}

func has(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
