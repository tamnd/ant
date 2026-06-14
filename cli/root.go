// Package cli builds the ant command tree: a URI front door over every kit
// domain. ant composes the single-site libraries rather than replacing them, so
// a domain becomes addressable by registering with kit from its own package's
// init. The blank imports below are the whole coupling: enabling a domain is one
// line, exactly as a database/sql program enables a driver.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/tamnd/ant/ant"

	// Domain drivers. Each registers itself with kit on init; ant drives them as
	// libraries in one static binary, never as subprocesses.
	_ "github.com/tamnd/goodread-cli/goodread"
	_ "github.com/tamnd/wikipedia-cli/wiki"
	_ "github.com/tamnd/x-cli/x"
	_ "github.com/tamnd/ytb-cli/youtube"
)

// Build metadata, set via -ldflags at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// shared persistent flags, read by every verb through engineFrom.
var (
	flagData   string // --data: the on-disk URI-tree root
	flagOutput string // --output/-o: json|md
)

// Root builds the root command and its subtree.
func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "ant",
		Short: "Every structured resource is a URI",
		Long: `ant is a URI front door over the tamnd site CLIs. It dereferences a
resource URI to a record, follows typed links across sites, and materializes a
slice of the graph to disk as the URI tree, regardless of which site a name
lives on.

  ant get goodreads://book/2767052
  ant resolve "https://x.com/nasa"
  ant links goodreads://book/2767052
  ant export goodreads://author/153394 --follow 1`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	pf := root.PersistentFlags()
	pf.StringVar(&flagData, "data", "", "data-tree root (default $HOME/data, or $ANT_DATA)")
	pf.StringVarP(&flagOutput, "output", "o", "json", "output format: json|md")

	root.AddCommand(
		newVersionCmd(),
		newGetCmd(),
		newLsCmd(),
		newCatCmd(),
		newLinksCmd(),
		newResolveCmd(),
		newURLCmd(),
		newOpenCmd(),
		newExportCmd(),
		newImportCmd(),
		newLLCmd(),
		newGraphCmd(),
		newServeCmd(),
		newMCPCmd(),
		newDomainsCmd(),
	)
	return root
}

// engineFrom builds the Engine from the shared persistent flags.
func engineFrom() (*ant.Engine, error) {
	var opts []ant.Option
	if flagData != "" {
		opts = append(opts, ant.WithRoot(flagData))
	}
	return ant.New(opts...)
}
