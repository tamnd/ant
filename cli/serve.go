package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

func newServeCmd() *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Dereference server: HTTP GET on a URI returns the record",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			e, err := engineFrom()
			if err != nil {
				return err
			}
			srv := &http.Server{
				Addr:              addr,
				Handler:           dereferenceMux(e),
				ReadHeaderTimeout: 10 * time.Second,
			}
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(c.OutOrStdout(), "ant serve listening on %s\n", ln.Addr()); err != nil {
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

// dereferenceMux turns the URI namespace into dereferenceable linked data: a raw
// URI path returns its record, and the query endpoints cover resolve/ls/links/url.
func dereferenceMux(e *ant.Engine) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("/resolve", func(w http.ResponseWriter, r *http.Request) {
		u, err := e.Resolve(r.URL.Query().Get("input"), r.URL.Query().Get("on"))
		if err != nil {
			httpErr(w, http.StatusBadRequest, err)
			return
		}
		httpJSON(w, map[string]string{"uri": u.String()})
	})

	mux.HandleFunc("/url", func(w http.ResponseWriter, r *http.Request) {
		u, err := kit.ParseURI(r.URL.Query().Get("uri"))
		if err != nil {
			httpErr(w, http.StatusBadRequest, err)
			return
		}
		loc, err := e.URL(u)
		if err != nil {
			httpErr(w, http.StatusBadRequest, err)
			return
		}
		httpJSON(w, map[string]string{"url": loc})
	})

	mux.HandleFunc("/ls", func(w http.ResponseWriter, r *http.Request) {
		u, err := kit.ParseURI(r.URL.Query().Get("uri"))
		if err != nil {
			httpErr(w, http.StatusBadRequest, err)
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("n"))
		envs, err := e.List(r.Context(), u, limit)
		if err != nil {
			httpErr(w, http.StatusBadGateway, err)
			return
		}
		httpJSON(w, envs)
	})

	mux.HandleFunc("/links", func(w http.ResponseWriter, r *http.Request) {
		u, err := kit.ParseURI(r.URL.Query().Get("uri"))
		if err != nil {
			httpErr(w, http.StatusBadRequest, err)
			return
		}
		links, err := e.Links(r.Context(), u)
		if err != nil {
			httpErr(w, http.StatusBadGateway, err)
			return
		}
		out := make([]string, 0, len(links))
		for _, lu := range links {
			out = append(out, lu.String())
		}
		httpJSON(w, out)
	})

	// The catch-all: a raw URI in the path (GET /goodreads://book/2767052).
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.URL.Path, "/")
		if raw == "" {
			httpJSON(w, map[string]any{"service": "ant", "domains": e.Domains()})
			return
		}
		u, err := kit.ParseURI(raw)
		if err != nil {
			httpErr(w, http.StatusBadRequest, err)
			return
		}
		env, err := e.Get(r.Context(), u)
		if err != nil {
			httpErr(w, http.StatusBadGateway, err)
			return
		}
		httpJSON(w, env)
	})

	return mux
}

func httpJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, v)
}

func httpErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = writeJSON(w, map[string]string{"error": err.Error()})
}
