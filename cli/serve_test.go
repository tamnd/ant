package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/ant/web"
)

// The drivers are blank-imported by root.go, so the cli test binary has the
// goodreads, x, wikipedia and youtube domains registered. That is enough to
// exercise the console's offline paths without touching the network.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	e, err := ant.New(ant.WithRoot(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	console, err := web.New(e, web.Build{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return console.Handler()
}

// A request without an Accept: text/html header negotiates to JSON, so the
// console keeps answering scripts the way ant serve always has.
func TestServeNamedEndpoints(t *testing.T) {
	h := newTestHandler(t)

	cases := []struct {
		path, want string
	}{
		{"/healthz", "ok"},
		{"/resolve?input=https://x.com/nasa", `"x://user/nasa"`},
		{"/url?uri=x://user/nasa", `"https://x.com/nasa"`},
		{"/api/resolve?input=https://x.com/nasa", `"x://user/nasa"`},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: code %d, want 200 (%s)", c.path, rec.Code, rec.Body.String())
			continue
		}
		if !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: body %q does not contain %q", c.path, rec.Body.String(), c.want)
		}
	}
}

// A browser (Accept: text/html) gets the GUI, not JSON, on the same URL.
func TestServeNegotiatesHTML(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/ code %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("/ content-type %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Error("/ did not render the HTML shell")
	}
}

// The regression this guards: a resource URI in the path carries a "//", and an
// http.ServeMux would 301-redirect it (collapsing the slashes) before any handler
// ran. The hand-rolled router must instead reach the dereference handler. We can
// assert that offline because the only failure left is the network fetch, which
// is a 502 gateway error, never a 301.
func TestServeRawURIPathIsNotRedirected(t *testing.T) {
	h := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x://status/20", nil))
	if rec.Code == http.StatusMovedPermanently || rec.Code == http.StatusPermanentRedirect {
		t.Fatalf("raw URI path was redirected (code %d); the // was collapsed", rec.Code)
	}
}
