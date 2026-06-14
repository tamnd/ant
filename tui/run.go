// Package tui is the ant terminal console: a full-screen, keyboard-driven
// browser over the whole resource-URI namespace, built on Bubble Tea v2. It is
// the third human surface beside the CLI and the web console, and like the web
// console it adds no data capability of its own: every screen is a thin render of
// an ant.Engine method, reached through the Deref seam, so the program is fully
// testable against a network-free fake (8000_ant_tui §6, §16).
package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/tamnd/any-cli/kit"
)

// Build is the binary's release identity, surfaced on the dashboard and the help
// overlay. It mirrors web.Build so the command wiring passes the same value.
type Build struct {
	Version string
	Commit  string
	Date    string
}

// Run starts the terminal console over e. If initial is non-empty it is resolved
// (offline, the same Resolve the omnibox uses) and opened on launch, so
// `ant tui goodreads://book/2767052` lands straight on the record. Run blocks
// until the user quits or ctx is cancelled by the signal handler.
func Run(ctx context.Context, e Deref, b Build, initial string) error {
	d := &deps{
		e:     e,
		ctx:   ctx,
		keys:  newKeys(),
		sty:   newStyles(true), // refined on the first BackgroundColorMsg
		build: b,
	}

	var initURI *kit.URI
	if initial != "" {
		u, err := e.Resolve(initial, "")
		if err != nil {
			return err
		}
		initURI = &u
	}

	p := tea.NewProgram(newApp(d, initURI), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}
