package tui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderDataShowsFields(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"data": map[string]any{"name": "Widget 42", "id": "42"}})
	out := renderData(raw, newStyles(true), 60)
	if !strings.Contains(out, "Widget 42") {
		t.Fatalf("renderData should show the value:\n%s", out)
	}
}

func TestRenderJSONIndents(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"a": 1})
	out := renderJSON(raw, newStyles(true))
	if !strings.Contains(out, "{") || !strings.Contains(out, "\"a\"") {
		t.Fatalf("renderJSON should pretty-print:\n%s", out)
	}
}

func TestFlattenLinksSortedFollowable(t *testing.T) {
	rows := flattenLinks(map[string][]string{
		"sequel_id": {"demo://book/2"},
		"author_id": {"demo://author/9"},
		"not_a_uri": {"just text"},
	})
	if len(rows) != 2 {
		t.Fatalf("only parseable URIs are followable, got %d", len(rows))
	}
	if rows[0].Field != "author_id" {
		t.Fatalf("rows should be sorted by field, got %q first", rows[0].Field)
	}
}

func TestHumanize(t *testing.T) {
	if got := humanize("maker_id"); got != "Maker id" {
		t.Fatalf("humanize(maker_id) = %q", got)
	}
}

func TestRenderMarkdownCached(t *testing.T) {
	body := "# Title\n\nSome *text*."
	a := renderMarkdown(body, true, 60)
	b := renderMarkdown(body, true, 60)
	if a == "" {
		t.Fatal("renderMarkdown should produce output")
	}
	if a != b {
		t.Fatal("a cached render should be byte-identical")
	}
}

func TestSchemeStyleStable(t *testing.T) {
	sty := newStyles(true)
	// A known scheme and an unknown one both resolve to a usable style.
	if sty.schemeStyle("goodreads").Render("x") == "" {
		t.Fatal("known scheme should render")
	}
	if sty.schemeStyle("totally-unknown").Render("x") == "" {
		t.Fatal("unknown scheme should fall back, not vanish")
	}
}
