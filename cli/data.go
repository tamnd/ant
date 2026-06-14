package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tamnd/any-cli/kit"
)

func newExportCmd() *cobra.Command {
	var (
		follow int
		to     string
		asMd   bool
	)
	cmd := &cobra.Command{
		Use:   "export <uri>",
		Short: "Materialize a resource (and --follow links) to the data tree",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if to != "" {
				flagData = to
			}
			e, err := engineFrom()
			if err != nil {
				return err
			}
			u, err := kit.ParseURI(args[0])
			if err != nil {
				return err
			}
			rep, err := e.Export(c.Context(), u, follow, asMd || flagOutput == "md")
			if err != nil {
				return err
			}
			return writeJSON(c.OutOrStdout(), rep)
		},
	}
	f := cmd.Flags()
	f.IntVar(&follow, "follow", 0, "follow links to this depth (0 = just the root)")
	f.StringVar(&to, "to", "", "data-tree root to write under (default --data / $HOME/data)")
	f.BoolVar(&asMd, "md", false, "also write a Markdown body file for records that have one")
	return cmd
}

func newImportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <path>",
		Short: "Read an exported record file back as a record",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			env, err := e.Import(args[0])
			if err != nil {
				return err
			}
			return writeJSON(c.OutOrStdout(), env)
		},
	}
}

func newLLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ll [<uri-prefix>]",
		Short: "List what is already on disk under a URI prefix",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			prefix := ""
			if len(args) == 1 {
				prefix = args[0]
			}
			uris, err := e.LL(prefix)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			for _, u := range uris {
				if _, err := fmt.Fprintln(out, u); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newGraphCmd() *cobra.Command {
	var (
		depth  int
		format string
	)
	cmd := &cobra.Command{
		Use:   "graph <uri>",
		Short: "Walk links to depth N and print the subgraph (dot|json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			u, err := kit.ParseURI(args[0])
			if err != nil {
				return err
			}
			g, err := e.Walk(c.Context(), u, depth)
			if err != nil {
				return err
			}
			if format == "dot" {
				_, err := c.OutOrStdout().Write([]byte(g.Dot()))
				return err
			}
			return writeJSON(c.OutOrStdout(), g)
		},
	}
	f := cmd.Flags()
	f.IntVar(&depth, "depth", 1, "how many link hops to walk")
	f.StringVar(&format, "format", "json", "output format: json|dot")
	return cmd
}
