package web

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// nonceKey is the request-context key under which the per-response CSP nonce is
// stashed, so the inline theme-init script can carry it (8000_ant_serve §13).
type nonceKey struct{}

// wantsJSON decides the representation for a negotiated route. A browser sends
// Accept: text/html and gets the GUI; everything else (curl, the test harness,
// other programs) gets the JSON that ant serve has always returned. The /api
// prefix and ?format= are explicit overrides (8000_ant_serve §5).
func wantsJSON(r *http.Request) bool {
	if strings.HasPrefix(trimLeadingSlash(r.URL.Path), "api/") {
		return true
	}
	switch r.URL.Query().Get("format") {
	case "json":
		return true
	case "html":
		return false
	}
	return !strings.Contains(r.Header.Get("Accept"), "text/html")
}

// statusFor maps an engine error to an HTTP status using the shared kit error
// taxonomy, so the console reports the same distinctions the CLI exit codes do
// (8000_ant_serve §5.1).
func statusFor(err error) int {
	switch errs.KindOf(err) {
	case errs.KindUsage:
		return http.StatusBadRequest
	case errs.KindNoResults, errs.KindNotFound:
		return http.StatusNotFound
	case errs.KindNeedAuth, errs.KindRateLimited, errs.KindNetwork, errs.KindUnsupported:
		return http.StatusBadGateway
	default:
		return http.StatusBadGateway
	}
}

// writeJSON marshals v as indented JSON with the right content type and status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// writeJSONErr writes the {"error": ...} body the JSON API has always used.
func writeJSONErr(w http.ResponseWriter, err error) {
	writeJSON(w, statusFor(err), map[string]string{"error": err.Error()})
}

// secureHeaders sets the console's security headers on every response. The only
// cross-origin load allowed is an <img> (record thumbnails); the sole inline
// script is the theme init, which carries this response's nonce
// (8000_ant_serve §13).
func secureHeaders(w http.ResponseWriter, nonce string) {
	w.Header().Set("Content-Security-Policy",
		"default-src 'none'; base-uri 'none'; form-action 'self'; "+
			"img-src 'self' https: data:; style-src 'self'; "+
			"script-src 'self' 'nonce-"+nonce+"'; connect-src 'self'; font-src 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
}

// cacheForever marks the embedded assets immutable; their URLs carry the build
// commit as a ?v= cache-buster so a new release invalidates them.
func cacheForever(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		h.ServeHTTP(w, r)
	})
}

// newNonce returns a fresh base64 CSP nonce.
func newNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return base64.StdEncoding.EncodeToString(b[:])
}

// trimLeadingSlash drops a single leading slash for first-segment routing.
func trimLeadingSlash(p string) string { return strings.TrimPrefix(p, "/") }

// firstSegment returns the path up to the first "/". A resource URI like
// "x://status/20" has first segment "x:", so it falls through to the resource
// handler rather than a named route.
func firstSegment(path string) string {
	if i := strings.IndexByte(path, '/'); i >= 0 {
		return path[:i]
	}
	return path
}

// isSchemeSegment reports whether a first segment is a URI scheme (ends in ':'
// with a valid scheme token), e.g. "x:" or "goodreads:".
func isSchemeSegment(seg string) bool {
	s, ok := strings.CutSuffix(seg, ":")
	if !ok || s == "" {
		return false
	}
	for i, r := range s {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if i == 0 && !isLower {
			return false
		}
		if !isLower && !isDigit {
			return false
		}
	}
	return true
}
