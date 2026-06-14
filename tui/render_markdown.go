package tui

import (
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/glamour"
)

// Body prose is Markdown, rendered with glamour to match the web console's
// goldmark pass (8000_ant_tui §8.3). The render is the one genuinely expensive
// formatting step, so it runs off the render loop as a command and its result is
// memoized per (theme, width, body): re-entering a record, or scrolling back to
// it, reuses the cached frame instead of re-rendering.

var (
	mdMu    sync.Mutex
	mdCache = map[string]string{}
)

// renderMarkdown formats body for the terminal at the given width and theme,
// caching the result. Unsafe raw HTML is dropped by glamour's sanitizer, the
// same guarantee the web console gets from never setting goldmark's WithUnsafe.
func renderMarkdown(body string, dark bool, width int) string {
	if width < 20 {
		width = 20
	}
	style := "light"
	if dark {
		style = "dark"
	}
	ck := style + ":" + itoa(width) + ":" + body
	mdMu.Lock()
	if out, ok := mdCache[ck]; ok {
		mdMu.Unlock()
		return out
	}
	mdMu.Unlock()

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return body
	}
	out, err := r.Render(body)
	if err != nil {
		return body
	}
	mdMu.Lock()
	mdCache[ck] = out
	mdMu.Unlock()
	return out
}

// renderBodyCmd renders a body off the render loop and returns it tagged with key
// so the requesting screen can install it when it arrives.
func renderBodyCmd(key, body string, dark bool, width int) tea.Cmd {
	return func() tea.Msg {
		return renderedMsg{Key: key, Markdown: renderMarkdown(body, dark, width)}
	}
}
