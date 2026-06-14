package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/ant/ant"
)

// The drivers are blank-imported by root.go, so the cli test binary has the
// goodreads and x domains registered. That is enough to exercise the router's
// offline paths without touching the network.
func newTestEngine(t *testing.T) *ant.Engine {
	t.Helper()
	e, err := ant.New(ant.WithRoot(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestServeNamedEndpoints(t *testing.T) {
	h := dereferenceMux(newTestEngine(t))

	cases := []struct {
		path, want string
	}{
		{"/healthz", "ok"},
		{"/resolve?input=https://x.com/nasa", `"x://user/nasa"`},
		{"/url?uri=x://user/nasa", `"https://x.com/nasa"`},
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

// The regression this guards: a resource URI in the path carries a "//", and an
// http.ServeMux would 301-redirect it (collapsing the slashes) before any handler
// ran. The hand-rolled router must instead reach the dereference handler. We can
// assert that offline because the only failure left is the network fetch, which
// is a 502 gateway error, never a 301.
func TestServeRawURIPathIsNotRedirected(t *testing.T) {
	h := dereferenceMux(newTestEngine(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x://status/20", nil))
	if rec.Code == http.StatusMovedPermanently || rec.Code == http.StatusPermanentRedirect {
		t.Fatalf("raw URI path was redirected (code %d); the // was collapsed", rec.Code)
	}
}

func TestFirstSegment(t *testing.T) {
	cases := map[string]string{
		"healthz":            "healthz",
		"resolve":            "resolve",
		"x://status/20":      "x:",
		"goodreads://book/1": "goodreads:",
		"":                   "",
	}
	for in, want := range cases {
		if got := firstSegment(in); got != want {
			t.Errorf("firstSegment(%q) = %q, want %q", in, got, want)
		}
	}
}
