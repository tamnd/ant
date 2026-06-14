package cli

import (
	"github.com/spf13/cobra"

	"github.com/tamnd/ant/tui"
)

func newTUICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui [uri]",
		Short: "Full-screen terminal browser over the URI namespace",
		Long: `tui opens the ant terminal console: a full-screen, keyboard-driven browser
over the whole resource-URI namespace. It is the third human surface beside the
CLI and the web console, sharing their vocabulary and keymap. Every screen is a
thin render of an Engine method, so it follows links, lists members, walks the
graph, and browses the on-disk cache without leaving the terminal.

  ant tui
  ant tui goodreads://book/2767052
  ant tui "https://x.com/nasa"

Press ? for the keymap, : to jump to any record, q to quit.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			// Warm the in-memory listing index off the render loop, so the first
			// dashboard or browse count comes from memory, not a cold walk.
			go e.WarmIndex()
			initial := ""
			if len(args) == 1 {
				initial = args[0]
			}
			return tui.Run(c.Context(), e, tui.Build{Version: Version, Commit: Commit, Date: Date}, initial)
		},
	}
	return cmd
}
