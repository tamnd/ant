package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/tamnd/any-cli/kit"
)

// App is the root model: it owns the screen stack, the omnibox, the help overlay,
// and the chrome (title bar + status bar). Screens never touch the stack
// directly; they emit navigate/push/back messages and the App is the single
// place that mutates it, which is what makes back/forward restore a screen
// exactly as it was left (8000_ant_tui §5, §12).
type App struct {
	d        *deps
	stack    []Screen // the back-stack; stack[0] is always the dashboard, top is active
	fwd      []Screen // popped screens, for forward
	omni     omnibox
	helpOpen bool
	spin     spinner.Model
	w, h     int
	toast    string
	toastErr bool
	initURI  *kit.URI // optional record to open after the first size arrives
}

// newApp seeds the stack with the dashboard. If initURI is set, the App opens it
// once sized, so back from it returns to the dashboard.
func newApp(d *deps, initURI *kit.URI) *App {
	return &App{
		d:       d,
		spin:    spinner.New(spinner.WithSpinner(spinner.Dot)),
		omni:    newOmnibox(d),
		stack:   []Screen{newDashboardScreen(d)},
		initURI: initURI,
	}
}

func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.spin.Tick, tea.RequestBackgroundColor, a.active().Init()}
	if a.initURI != nil {
		u := *a.initURI
		cmds = append(cmds, func() tea.Msg { return navigateMsg{URI: u} })
	}
	return tea.Batch(cmds...)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
		a.omni.setWidth(msg.Width)
		return a, a.resizeAll()

	case tea.BackgroundColorMsg:
		a.d.sty = newStyles(msg.IsDark())
		return a, a.resizeAll()

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spin, cmd = a.spin.Update(msg)
		return a, cmd

	case tea.KeyPressMsg:
		return a.onKey(msg)

	case navigateMsg:
		return a, a.open(newResourceScreen(a.d, msg.URI, msg.Refresh))
	case pushMsg:
		return a, a.open(msg.Screen)
	case replaceMsg:
		return a, a.replace(msg.Screen)
	case backMsg:
		a.back()
		return a, nil
	case forwardMsg:
		return a, a.forward()
	case homeMsg:
		a.goHome()
		return a, nil

	case resolvedMsg:
		if msg.Err != nil {
			return a, toastCmd("could not resolve: "+msg.Err.Error(), true)
		}
		return a, a.open(newResourceScreen(a.d, msg.URI, false))

	case exportedMsg:
		if msg.Err != nil {
			return a, toastCmd("export failed: "+msg.Err.Error(), true)
		}
		return a, toastCmd(fmt.Sprintf("exported %d records under %s", len(msg.Report.Written), msg.Report.Root), false)

	case toastMsg:
		a.toast, a.toastErr = msg.Text, msg.IsErr
		return a, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearToastMsg{} })
	case clearToastMsg:
		a.toast = ""
		return a, nil

	default:
		// Data results (fetched/listed/walked/searched/ll/rendered) are broadcast
		// to every screen so the one waiting on the key installs it, even a
		// background screen warmed by prefetch (8000_ant_tui §12).
		return a, a.broadcast(msg)
	}
}

func (a *App) View() tea.View {
	if a.w == 0 || a.h == 0 {
		v := tea.NewView("")
		v.AltScreen = true
		return v
	}
	bw, bh := a.bodySize()
	var body string
	if a.helpOpen {
		body = renderHelp(a.d.sty, bw, bh, a.helpGroups())
	} else {
		body = a.active().View(bw, bh)
	}
	content := strings.Join([]string{a.titleBar(), padLines(body, bw, bh), a.bottomBar()}, "\n")
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// --- key handling -----------------------------------------------------------

func (a *App) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// The omnibox, while open, swallows every key.
	if a.omni.active {
		var cmd tea.Cmd
		a.omni, cmd = a.omni.update(msg)
		return a, cmd
	}
	// Help overlay: any key dismisses it.
	if a.helpOpen {
		a.helpOpen = false
		return a, nil
	}
	// ctrl+c always quits, even mid-edit.
	if msg.String() == "ctrl+c" {
		return a, tea.Quit
	}
	// A screen editing text claims every key; only ctrl+c (above) escapes it.
	if a.active().Capturing() {
		return a, a.routeToScreen(msg)
	}

	k := a.d.keys
	switch {
	case key.Matches(msg, k.Quit):
		return a, tea.Quit
	case key.Matches(msg, k.Help):
		a.helpOpen = true
		return a, nil
	case key.Matches(msg, k.Omni):
		seed := ""
		if u, ok := a.active().Subject(); ok {
			seed = u.String()
		}
		return a, a.omni.open(seed)
	case key.Matches(msg, k.Theme):
		a.d.sty = newStyles(!a.d.sty.dark)
		return a, a.resizeAll()
	case key.Matches(msg, k.Home):
		a.goHome()
		return a, nil
	case key.Matches(msg, k.Forward):
		return a, a.forward()
	case key.Matches(msg, k.Back):
		if len(a.stack) > 1 {
			a.back()
			return a, nil
		}
		return a, a.routeToScreen(msg)
	default:
		return a, a.routeToScreen(msg)
	}
}

func (a *App) routeToScreen(msg tea.Msg) tea.Cmd {
	ns, cmd := a.active().Update(msg)
	a.setActive(ns)
	return cmd
}

// --- stack operations -------------------------------------------------------

func (a *App) active() Screen     { return a.stack[len(a.stack)-1] }
func (a *App) setActive(s Screen) { a.stack[len(a.stack)-1] = s }
func (a *App) bodySize() (int, int) {
	bh := a.h - 2
	if bh < 1 {
		bh = 1
	}
	return a.w, bh
}

// open sizes a new screen, pushes it, clears the forward stack, and runs its
// Init. A zero current size is fine: the next resizeAll will paint it.
func (a *App) open(s Screen) tea.Cmd {
	bw, bh := a.bodySize()
	s, _ = s.Update(tea.WindowSizeMsg{Width: bw, Height: bh})
	a.stack = append(a.stack, s)
	a.fwd = nil
	return s.Init()
}

// replace swaps the top screen in place (an in-place refresh that adds no back
// step).
func (a *App) replace(s Screen) tea.Cmd {
	bw, bh := a.bodySize()
	s, _ = s.Update(tea.WindowSizeMsg{Width: bw, Height: bh})
	a.setActive(s)
	return s.Init()
}

func (a *App) back() {
	if len(a.stack) <= 1 {
		return
	}
	last := a.stack[len(a.stack)-1]
	a.stack = a.stack[:len(a.stack)-1]
	a.fwd = append(a.fwd, last)
}

func (a *App) forward() tea.Cmd {
	if len(a.fwd) == 0 {
		return nil
	}
	s := a.fwd[len(a.fwd)-1]
	a.fwd = a.fwd[:len(a.fwd)-1]
	bw, bh := a.bodySize()
	s, cmd := s.Update(tea.WindowSizeMsg{Width: bw, Height: bh})
	a.stack = append(a.stack, s)
	return cmd
}

func (a *App) goHome() {
	a.fwd = nil
	a.stack = a.stack[:1]
}

// broadcast feeds a message to every screen in the stack and forward list,
// collecting their commands.
func (a *App) broadcast(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i := range a.stack {
		ns, cmd := a.stack[i].Update(msg)
		a.stack[i] = ns
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	for i := range a.fwd {
		ns, cmd := a.fwd[i].Update(msg)
		a.fwd[i] = ns
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) resizeAll() tea.Cmd {
	bw, bh := a.bodySize()
	return a.broadcast(tea.WindowSizeMsg{Width: bw, Height: bh})
}

// --- chrome -----------------------------------------------------------------

func (a *App) titleBar() string {
	sty := a.d.sty
	left := sty.Crumb.Render("ant") + sty.Crumb.Render(" › ") + sty.Title.Render(a.active().Title())
	if len(a.stack) > 1 {
		left += sty.Muted.Render(fmt.Sprintf("  [%d]", len(a.stack)))
	}
	right := ""
	if a.active().Loading() {
		right = a.spin.View() + sty.Muted.Render("loading")
	} else if a.active().Cached() {
		right = sty.OK.Render("● cached")
	}
	return bar(left, right, a.w)
}

func (a *App) bottomBar() string {
	sty := a.d.sty
	if a.omni.active {
		return clampLine(a.omni.View(sty), a.w)
	}
	if a.toast != "" {
		st := sty.OK
		if a.toastErr {
			st = sty.Err
		}
		return clampLine(st.Render(a.toast), a.w)
	}
	return clampLine(shortHelpLine(sty, a.active().ShortHelp(), a.d.keys), a.w)
}

func (a *App) helpGroups() [][]key.Binding {
	groups := a.active().FullHelp()
	groups = append(groups, globalHelp(a.d.keys))
	return groups
}

// --- chrome helpers ---------------------------------------------------------

// bar lays left at the start and right at the end of a w-wide line, clamping the
// left side first when the two would collide.
func bar(left, right string, w int) string {
	lw, rw := lipgloss.Width(left), lipgloss.Width(right)
	if lw+rw+1 > w {
		left = truncateANSI(left, max(0, w-rw-1))
		lw = lipgloss.Width(left)
	}
	gap := w - lw - rw
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// shortHelpLine renders the active screen's footer hints, always closing with the
// help key so '?' is discoverable from anywhere.
func shortHelpLine(sty *styles, bs []key.Binding, keys keyMap) string {
	bs = append(append([]key.Binding(nil), bs...), keys.Help)
	var parts []string
	for _, b := range bs {
		if !b.Enabled() {
			continue
		}
		parts = append(parts, sty.Title.Render(b.Help().Key)+" "+sty.Muted.Render(b.Help().Desc))
	}
	return strings.Join(parts, sty.Crumb.Render(" · "))
}

// toastCmd flashes a transient status line.
func toastCmd(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return toastMsg{Text: text, IsErr: isErr} }
}
