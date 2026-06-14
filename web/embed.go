package web

import (
	"embed"
	"mime"
)

// files holds the whole console: the html/template sources under templates/ and
// the static CSS/JS/SVG/fonts under assets/. Embedding them keeps ant a single
// static binary: there is no asset directory to ship next to it and nothing to
// fetch at runtime, the embedded Geist Mono face included (8000_ant_serve §2, WC1).
//
//go:embed templates assets
var files embed.FS

// Register the woff2 type so the embedded font serves as font/woff2 regardless
// of the host's MIME database; without it some systems fall back to a generic
// type and the browser declines to use the font.
func init() { _ = mime.AddExtensionType(".woff2", "font/woff2") }
