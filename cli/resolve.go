package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/tamnd/any-cli/kit"
)

func newResolveCmd() *cobra.Command {
	var on string
	cmd := &cobra.Command{
		Use:   "resolve <input>",
		Short: "Normalize any id, URL, or URI to its canonical URI",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			u, err := e.Resolve(args[0], on)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), u.String())
			return err
		},
	}
	cmd.Flags().StringVar(&on, "on", "", "resolve a bare id or @handle within this domain (scheme)")
	return cmd
}

func newURLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url <uri>",
		Short: "The live https location for a URI (inverse of resolve)",
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
			loc, err := e.URL(u)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), loc)
			return err
		},
	}
}

func newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open <uri>",
		Short: "Open a URI's live URL in a browser",
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
			loc, err := e.URL(u)
			if err != nil {
				return err
			}
			return openBrowser(loc)
		},
	}
}

// openBrowser opens url with the platform's default handler.
func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

func newDomainsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "domains",
		Short: "List the registered domains ant can address",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			return writeJSON(c.OutOrStdout(), e.Domains())
		},
	}
}
