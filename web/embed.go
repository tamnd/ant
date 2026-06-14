package web

import "embed"

// files holds the whole console: the html/template sources under templates/ and
// the static CSS/JS/SVG under assets/. Embedding them keeps ant a single static
// binary — there is no asset directory to ship next to it and nothing to fetch
// at runtime (8000_ant_serve §2, WC1).
//
//go:embed templates assets
var files embed.FS
