package tui

import (
	"hash/fnv"
	"image/color"

	"charm.land/lipgloss/v2"
)

// styles is the resolved palette and the named lipgloss styles every screen
// draws with. It is built once from the detected background (8000_ant_tui §3.4)
// and rebuilt only when the theme is toggled, so screens never touch color
// literals directly: a theme change is one place.
type styles struct {
	dark bool

	fg      color.Color
	muted   color.Color
	border  color.Color
	surface color.Color
	accent  color.Color
	errc    color.Color
	okc     color.Color

	Base      lipgloss.Style // body text
	Muted     lipgloss.Style // secondary text, crumbs
	Title     lipgloss.Style // bold heading
	Key       lipgloss.Style // a kv field name
	Err       lipgloss.Style // an error line
	OK        lipgloss.Style // a success line / cache badge
	Sel       lipgloss.Style // the cursor row
	Crumb     lipgloss.Style // a breadcrumb segment
	Pane      lipgloss.Style // an unfocused bordered pane
	PaneFocus lipgloss.Style // the focused bordered pane (accent border)
}

// newStyles resolves the palette through lipgloss.LightDark and builds the named
// styles. dark comes from lipgloss.HasDarkBackground at startup (§13.1).
func newStyles(dark bool) *styles {
	ld := lipgloss.LightDark(dark)
	c := func(light, darkc string) color.Color { return ld(lipgloss.Color(light), lipgloss.Color(darkc)) }

	s := &styles{dark: dark}
	s.fg = c("#18181b", "#fafafa")
	s.muted = c("#71717a", "#a1a1aa")
	s.border = c("#d4d4d8", "#3f3f46")
	s.surface = c("#f4f4f5", "#27272a")
	s.accent = c("#18181b", "#fafafa")
	s.errc = c("#dc2626", "#f87171")
	s.okc = c("#16a34a", "#4ade80")

	s.Base = lipgloss.NewStyle().Foreground(s.fg)
	s.Muted = lipgloss.NewStyle().Foreground(s.muted)
	s.Title = lipgloss.NewStyle().Foreground(s.fg).Bold(true)
	s.Key = lipgloss.NewStyle().Foreground(s.muted)
	s.Err = lipgloss.NewStyle().Foreground(s.errc)
	s.OK = lipgloss.NewStyle().Foreground(s.okc)
	s.Crumb = lipgloss.NewStyle().Foreground(s.muted)
	s.Sel = lipgloss.NewStyle().Foreground(s.fg).Background(s.surface).Bold(true)
	s.Pane = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(s.border)
	s.PaneFocus = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(s.accent)
	return s
}

// schemeHex mirrors the web console's per-domain accent map (8000_ant_serve §3.1)
// so the two surfaces agree on a domain's color. An unknown driver hashes into a
// stable fallback hue.
var schemeHex = map[string]string{
	"goodreads": "#d97706", "x": "#0ea5e9", "wikipedia": "#a1a1aa",
	"youtube": "#dc2626", "reddit": "#ea580c", "facebook": "#3b82f6",
	"bilibili": "#ec4899", "amazon": "#f59e0b", "archive": "#22c55e",
	"threads": "#a78bfa", "douban": "#16a34a", "xiaohongshu": "#f43f5e",
}

var fallbackPalette = []string{
	"#0ea5e9", "#22c55e", "#f59e0b", "#a78bfa",
	"#ec4899", "#14b8a6", "#f43f5e", "#84cc16",
}

// schemeColor resolves a scheme to its accent color: the fixed map, else a stable
// hash into the fallback palette so an unknown driver still gets a consistent dot.
func schemeColor(scheme string) color.Color {
	if hex, ok := schemeHex[scheme]; ok {
		return lipgloss.Color(hex)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(scheme))
	return lipgloss.Color(fallbackPalette[h.Sum32()%uint32(len(fallbackPalette))])
}

// schemeStyle is a foreground style in a scheme's accent, for badges and URIs.
func (s *styles) schemeStyle(scheme string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(schemeColor(scheme))
}
