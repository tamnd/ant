package web

import (
	"context"
	"sync"
	"time"
)

// jobPhase is the lifecycle of a background fetch.
type jobPhase int

const (
	jobPending jobPhase = iota // running, or queued behind the prefetch limiter
	jobReady                   // finished; value holds the result
	jobError                   // finished; err holds the failure
)

// graceWindow is how long a browser page waits for a just-started fetch to finish
// inline before it hands off to the loading screen. wait returns the instant the
// job completes, so a cache hit or a fast site renders well within it and never
// flashes a spinner; only a genuinely slow fetch shows one (8000_ant_serve §24).
// It is a var, not a const, only so a test can shrink it.
var graceWindow = 600 * time.Millisecond

const (
	// jobTimeout is the hard backstop on a single background fetch. A page never
	// blocks this long (the grace window hands off to the loading screen well
	// before), but a truly hung upstream must not leak a goroutine forever; when it
	// trips, the loading page surfaces a clean error rather than a blank "context
	// deadline exceeded".
	jobTimeout = 120 * time.Second
	// jobRetain is how long a finished job's result is kept, so a quick reload (and
	// the poller's last tick) reads it from memory instead of re-fetching.
	jobRetain = 5 * time.Minute
	// prefetchLimit bounds how many speculative link warm-ups run at once, so
	// opening a record with many links cannot stampede a site. Interactive fetches
	// do not draw from this pool, so a click is never queued behind a prefetch.
	prefetchLimit = 4
)

// runFn is the work a job runs: a context-aware fetch returning its result.
type runFn func(ctx context.Context) (any, error)

// job is one in-flight or recently finished fetch, deduplicated by key so several
// concurrent viewers of the same URI share a single upstream call.
type job struct {
	mu      sync.Mutex
	phase   jobPhase
	value   any
	err     error
	started time.Time
	ended   time.Time
	done    chan struct{}
}

// snapshot reads the job's current phase and result under its lock.
func (j *job) snapshot() (jobPhase, any, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.phase, j.value, j.err
}

// wait blocks until the job finishes or ctx is done, then returns the current
// snapshot. A ctx timeout leaves the job running (other waiters and the next visit
// still benefit); the caller sees jobPending and renders a loading screen.
func (j *job) wait(ctx context.Context) (jobPhase, any, error) {
	select {
	case <-j.done:
	case <-ctx.Done():
	}
	return j.snapshot()
}

// jobs is the console's fetch manager: a small dedup + background-run + retain
// layer in front of the Engine, so a page renders instantly (from cache or a
// loading screen) and the network happens off the request's critical path
// (8000_ant_serve §24).
type jobs struct {
	mu          sync.Mutex
	m           map[string]*job
	now         func() time.Time
	prefetchSem chan struct{}
}

func newJobs() *jobs {
	return &jobs{
		m:           map[string]*job{},
		now:         time.Now,
		prefetchSem: make(chan struct{}, prefetchLimit),
	}
}

// start returns the job for key, launching it when absent or when the last run
// finished long enough ago to be stale. The returned job may already hold a warm
// result or still be running; the caller waits on it with its own deadline.
// Interactive fetches use this path and never block on the prefetch limiter.
func (j *jobs) start(key string, run runFn) *job {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.sweepLocked()
	if existing, ok := j.m[key]; ok && !j.staleLocked(existing) {
		return existing
	}
	return j.launchLocked(key, run, nil)
}

// prefetch warms key in the background without returning a job: fire-and-forget,
// deduplicated against in-flight and warm results, and bounded by the prefetch
// limiter so a fan-out of links cannot stampede a site.
func (j *jobs) prefetch(key string, run runFn) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.sweepLocked()
	if existing, ok := j.m[key]; ok && !j.staleLocked(existing) {
		return
	}
	j.launchLocked(key, run, j.prefetchSem)
}

// peek returns the job for key without starting one, for the status endpoint.
func (j *jobs) peek(key string) (*job, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	jb, ok := j.m[key]
	return jb, ok
}

// launchLocked creates a job, registers it, and starts its worker. The worker runs
// on a background context (not the request's), so a viewer navigating away does
// not abort a fetch others are waiting on. With sem set, the worker waits for a
// slot before it begins, which is what bounds prefetch concurrency. j.mu must be
// held.
func (j *jobs) launchLocked(key string, run runFn, sem chan struct{}) *job {
	jb := &job{phase: jobPending, started: j.now(), done: make(chan struct{})}
	j.m[key] = jb
	go func() {
		if sem != nil {
			sem <- struct{}{}
			defer func() { <-sem }()
		}
		ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		defer cancel()
		v, err := run(ctx)
		jb.mu.Lock()
		if err != nil {
			jb.phase, jb.err = jobError, err
		} else {
			jb.phase, jb.value = jobReady, v
		}
		jb.ended = j.now()
		jb.mu.Unlock()
		close(jb.done)
	}()
	return jb
}

// staleLocked reports whether a finished job is old enough to re-run on the next
// request, so a result does not pin forever and a later visit eventually
// re-fetches. j.mu must be held.
func (j *jobs) staleLocked(jb *job) bool {
	jb.mu.Lock()
	defer jb.mu.Unlock()
	if jb.phase == jobPending {
		return false
	}
	return j.now().Sub(jb.ended) > jobRetain
}

// sweepLocked drops finished jobs past their retain window so the map cannot grow
// without bound over a long-lived server. It runs only once the map is sizeable,
// to keep the common request path allocation-free. j.mu must be held.
func (j *jobs) sweepLocked() {
	if len(j.m) < 256 {
		return
	}
	for key, jb := range j.m {
		if j.staleLocked(jb) {
			delete(j.m, key)
		}
	}
}
