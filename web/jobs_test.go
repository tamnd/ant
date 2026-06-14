package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tamnd/ant/ant"
	"github.com/tamnd/any-cli/kit"
)

// TestJobDedup proves that concurrent starts on the same key share one run, the
// property that makes N viewers of a slow URI cost a single upstream fetch.
func TestJobDedup(t *testing.T) {
	j := newJobs()
	var runs int32
	release := make(chan struct{})
	run := func(ctx context.Context) (any, error) {
		atomic.AddInt32(&runs, 1)
		<-release
		return "v", nil
	}
	a := j.start("k", run)
	b := j.start("k", run)
	if a != b {
		t.Fatal("start returned two different jobs for one key")
	}
	close(release)
	if ph, v, _ := a.wait(context.Background()); ph != jobReady || v != "v" {
		t.Fatalf("phase=%v value=%v, want ready/v", ph, v)
	}
	if got := atomic.LoadInt32(&runs); got != 1 {
		t.Fatalf("run executed %d times, want 1", got)
	}
}

// TestJobError records the failure on the job, for the error-state render.
func TestJobError(t *testing.T) {
	j := newJobs()
	jb := j.start("k", func(ctx context.Context) (any, error) {
		return nil, errors.New("boom")
	})
	ph, _, err := jb.wait(context.Background())
	if ph != jobError || err == nil || err.Error() != "boom" {
		t.Fatalf("phase=%v err=%v, want error/boom", ph, err)
	}
}

// TestJobWaitDeadlineLeavesItRunning proves a waiter that gives up sees pending
// while the job keeps running, so a browser falls through to the loading screen
// instead of killing a fetch other viewers (and the next visit) still want.
func TestJobWaitDeadlineLeavesItRunning(t *testing.T) {
	j := newJobs()
	release := make(chan struct{})
	jb := j.start("k", func(ctx context.Context) (any, error) {
		<-release
		return "done", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if ph, _, _ := jb.wait(ctx); ph != jobPending {
		t.Fatalf("early wait phase=%v, want pending", ph)
	}
	close(release)
	if ph, v, _ := jb.wait(context.Background()); ph != jobReady || v != "done" {
		t.Fatalf("final wait phase=%v value=%v, want ready/done", ph, v)
	}
}

// TestPrefetchBounded proves the prefetch limiter caps concurrency: with a limit
// of prefetchLimit, no more than that many warm-ups run at once.
func TestPrefetchBounded(t *testing.T) {
	j := newJobs()
	var inflight, peak int32
	release := make(chan struct{})
	run := func(ctx context.Context) (any, error) {
		n := atomic.AddInt32(&inflight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if n <= p || atomic.CompareAndSwapInt32(&peak, p, n) {
				break
			}
		}
		<-release
		atomic.AddInt32(&inflight, -1)
		return nil, nil
	}
	for i := 0; i < prefetchLimit+4; i++ {
		j.prefetch("k"+string(rune('a'+i)), run)
	}
	// Give the workers a moment to saturate the limiter.
	time.Sleep(30 * time.Millisecond)
	if got := atomic.LoadInt32(&peak); got > int32(prefetchLimit) {
		t.Fatalf("prefetch ran %d at once, want <= %d", got, prefetchLimit)
	}
	close(release)
}

// slowDeref is a Deref whose record fetch blocks until released and whose cache is
// always cold, so the resource page is forced down the background-job path.
type slowDeref struct {
	fakeDeref
	release chan struct{}
}

func (slowDeref) Lookup(kit.URI) (ant.Fetched, bool) { return ant.Fetched{}, false }
func (slowDeref) Cached(kit.URI) bool                { return false }
func (d slowDeref) Dereference(ctx context.Context, u kit.URI, refresh bool) (ant.Fetched, error) {
	select {
	case <-d.release:
		return d.fakeDeref.Dereference(ctx, u, refresh)
	case <-ctx.Done():
		return ant.Fetched{}, ctx.Err()
	}
}

// TestColdFetchShowsLoadingThenStatus drives the whole async path: a cold,
// slow record renders the loading screen (202 + poller), the status endpoint
// reports pending while the fetch runs, and ready once it finishes.
func TestColdFetchShowsLoadingThenStatus(t *testing.T) {
	defer func(prev time.Duration) { graceWindow = prev }(graceWindow)
	graceWindow = 15 * time.Millisecond

	d := slowDeref{release: make(chan struct{})}
	c, err := New(d, Build{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	h := c.Handler()

	// The page itself: a browser load returns the loading screen, not a hang.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/view?uri=demo://widget/42", nil)
	req.Header.Set("Accept", "text/html")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("loading page code=%d, want 202\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "spinner") || !strings.Contains(body, "/assets/poll.js") {
		t.Errorf("loading page missing spinner/poller:\n%s", body)
	}
	if !strings.Contains(body, "/status?") {
		t.Errorf("loading page missing status URL")
	}

	// The status endpoint: pending while the fetch is blocked.
	if got := statusPhase(t, h, "demo://widget/42"); got != "pending" {
		t.Fatalf("status while blocked = %q, want pending", got)
	}

	// Release the fetch and let it complete, then status flips to ready.
	close(d.release)
	deadline := time.Now().Add(2 * time.Second)
	for statusPhase(t, h, "demo://widget/42") != "ready" {
		if time.Now().After(deadline) {
			t.Fatal("status never became ready")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func statusPhase(t *testing.T, h http.Handler, uri string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status?op=get&uri="+uri, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status code=%d", rec.Code)
	}
	var got struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("status body %q: %v", rec.Body.String(), err)
	}
	return got.Status
}

// TestStatusUnknownJobReadsAsReady proves a poll for a job that was never started
// (or was swept) tells the poller to reload rather than spin forever.
func TestStatusUnknownJobReadsAsReady(t *testing.T) {
	c, err := New(fakeDeref{}, Build{Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if got := statusPhase(t, c.Handler(), "demo://never/started"); got != "ready" {
		t.Fatalf("unknown job status = %q, want ready", got)
	}
}
