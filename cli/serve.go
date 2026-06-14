package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/tamnd/ant/web"
)

func newServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Web console + dereference server over the URI namespace",
		Long: `serve runs the ant web console: a browser GUI over the whole resource-URI
namespace, server-rendered and styled like shadcn/ui. The same URLs answer with
JSON for scripts under content negotiation (or the /api/ prefix), so the GET-a-URI
contract ant serve has always offered is preserved.

  ant serve
  ant serve --addr :8080

Then open http://localhost:7777/ in a browser, or:

  curl http://localhost:7777/api/resolve?input=https://x.com/nasa
  curl -H 'Accept: application/json' http://localhost:7777/x://status/20`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			console, err := web.New(e, web.Build{Version: Version, Commit: Commit, Date: Date})
			if err != nil {
				return err
			}
			srv := &http.Server{
				Addr:              addr,
				Handler:           console.Handler(),
				ReadHeaderTimeout: 10 * time.Second,
			}
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(c.OutOrStdout(), "ant serve listening on http://%s\n", ln.Addr()); err != nil {
				return err
			}

			go func() {
				<-c.Context().Done()
				shctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = srv.Shutdown(shctx)
			}()

			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&addr, "addr", ":7777", "listen address")
	return cmd
}
