package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/sam0uly/spin/internal/template"
	"github.com/sam0uly/spin/internal/theme"
)

// hookItem adapts a template.HookView to the bubbles list.Item interface.
type hookItem struct {
	template.HookView
}

func (i hookItem) FilterValue() string { return i.Title() }

func (i hookItem) Title() string {
	if i.IsFile {
		return fmt.Sprintf("%s  %s", i.Phase, filepath.Base(i.File))
	}
	return fmt.Sprintf("%s  %s", i.Phase, i.Run)
}

func (i hookItem) Description() string {
	if i.IsFile {
		return i.File
	}
	return "inline command"
}

// runLineMsg delivers one chunk of streamed hook output.
type runLineMsg struct{ text string }

// runDoneMsg signals the hook run finished; err carries any failure.
type runDoneMsg struct{ err error }

const (
	hooksListContentW = 42
	hooksListBorderW  = 2 // rounded border left + right
	hooksPaneGap      = 1 // column between list and view boxes
	hooksViewBorderW  = 2 // rounded border left + right
	hooksViewMinW     = 20
)

// hooksBodyOverhead is horizontal space in the hooks body layout taken by
// everything except the view pane's content width: list content, list
// borders, the inter-pane gap, and view borders.
const hooksBodyOverhead = hooksListContentW + hooksListBorderW + hooksPaneGap + hooksViewBorderW

func hooksViewContentW(totalW int) int {
	return max(totalW-hooksBodyOverhead, hooksViewMinW)
}

const hookHintShort = "R run all + scaffold • ←/→ focus • q/esc quit"

// wrapForView hard-wraps s to the viewport width via ansi.Hardwrap, so
// styling is preserved and long unbroken tokens wrap instead of
// overflowing the pane.
func wrapForView(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Hardwrap(s, width, false)
}

// hooksModel renders the interactive hook review screen: a hook list
// on the left and a live output pane on the right. R opens a centered
// Run/Skip/Cancel modal that replaces the CLI trust prompt; Run or Skip
// executes the full scaffold (pre, render, post) with output streaming
// into the right pane.
type hooksModel struct {
	styles   *tuiStyles
	tpl      *template.Template
	ctx      context.Context
	dest     string
	name     string
	resolved map[string]any
	list     list.Model
	viewport viewport.Model
	hooks    []template.HookView
	width    int
	height   int
	focus    string // "list" or "view"
	selected int

	modalOpen   bool
	modalChoice int // 0 Run, 1 Skip

	running bool
	output  string
	didRun  bool // scaffold was executed by this model
	stream  chan string
	doneCh  chan error

	verbose   bool
	autoStart int // 0 none, 1 run, 2 skip
}

func newHooksModel(tpl *template.Template, styles *tuiStyles, width, height int, resolved map[string]any, ctx context.Context, dest, name string, noHooks, yes, verbose bool) hooksModel {
	items := template.CollectHooks(tpl)
	listItems := make([]list.Item, 0, len(items))
	for _, h := range items {
		listItems = append(listItems, hookItem{h})
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles = theme.ListItemStyles()
	listH := max(height-7, 1)
	l := list.New(listItems, delegate, hooksListContentW, listH)
	l.Styles = theme.ListStyles()
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.SetSize(hooksListContentW, listH)
	if len(listItems) > 0 {
		l.Select(0)
	}

	viewW := hooksViewContentW(width)
	vp := viewport.New(viewport.WithWidth(viewW), viewport.WithHeight(listH))

	m := hooksModel{
		styles:   styles,
		tpl:      tpl,
		ctx:      ctx,
		dest:     dest,
		name:     name,
		resolved: resolved,
		list:     l,
		viewport: vp,
		hooks:    items,
		width:    width,
		height:   height,
		focus:    "list",
		selected: -1,
		verbose:  verbose,
	}
	m.viewport.SetContent(wrapForView(m.selectedHookContent(m.list.Index()), m.viewport.Width()))
	m.viewport.GotoTop()
	if noHooks {
		m.autoStart = 2
	} else if yes {
		m.autoStart = 1
	}
	return m
}

// selectedHookContent builds the right-pane preview for the hook at
// idx: its inline command or script contents. Streamed output replaces
// it once the scaffold runs.
func (m hooksModel) selectedHookContent(idx int) string {
	var b strings.Builder
	if idx < 0 || idx >= len(m.hooks) {
		fmt.Fprintln(&b, "Select a hook to preview its command or script.")
		fmt.Fprintf(&b, "\n%s\n", hookHintShort)
		return b.String()
	}
	h := m.hooks[idx]
	header := fmt.Sprintf("Hook %d: %s", idx+1, h.Phase)
	if h.IsFile {
		data, err := os.ReadFile(h.File)
		body := ""
		if err != nil {
			body = fmt.Sprintf("(could not read %s: %v)", filepath.Base(h.File), err)
		} else {
			body = string(data)
		}
		fmt.Fprintf(&b, "%s  file  %s\n\n", header, filepath.Join("_"+h.Phase, filepath.Base(h.File)))
		b.WriteString(body)
	} else {
		fmt.Fprintf(&b, "%s  inline\n\n", header)
		fmt.Fprintf(&b, "  %s\n", h.Run)
	}
	fmt.Fprintf(&b, "\n%s\n", hookHintShort)
	return b.String()
}

func (m hooksModel) update(msg tea.Msg) (hooksModel, tea.Cmd) {
	if m.autoStart != 0 && !m.running && !m.modalOpen {
		skip := m.autoStart == 2
		m.autoStart = 0
		return m.startRun(skip)
	}

	switch msg := msg.(type) {
	case runLineMsg:
		m.output += msg.text
		// Cap accumulated output so rewrapping stays cheap; old lines
		// scroll out of view anyway.
		if len(m.output) > 16*1024 {
			if idx := strings.Index(m.output[4096:], "\n"); idx > 0 {
				m.output = m.output[4096+idx+1:]
			}
		}
		m.viewport.SetContent(wrapForView(m.output, m.viewport.Width()))
		m.viewport.GotoBottom()
		return m, m.listen()
	case runDoneMsg:
		m.running = false
		if msg.err != nil {
			m.output += "\n" + lipgloss.NewStyle().Foreground(theme.StatusError).Render("error: "+msg.err.Error())
			m.viewport.SetContent(wrapForView(m.output, m.viewport.Width()))
			m.viewport.GotoBottom()
			return m, nil
		}
		m.output += "\n" + lipgloss.NewStyle().Foreground(theme.StatusInfo).Render("done.")
		// Mirror the success summary in the pane; runNew reprints the
		// same lines on the restored terminal after quit.
		m.output += "\n"
		m.output += lipgloss.NewStyle().Foreground(theme.StatusSuccess).Render(
			fmt.Sprintf("INFO created %s at %s", m.name, m.dest))
		m.output += "\n" + lipgloss.NewStyle().Foreground(theme.TextDimmed).Render(
			fmt.Sprintf("cd %s", m.dest))
		m.viewport.SetContent(wrapForView(m.output, m.viewport.Width()))
		m.viewport.GotoBottom()
		return m, nil
	case tea.KeyPressMsg:
		if m.modalOpen {
			switch msg.String() {
			case "left", "h", "tab":
				m.modalChoice--
				if m.modalChoice < 0 {
					m.modalChoice = 2
				}
				return m, nil
			case "right", "l":
				m.modalChoice++
				if m.modalChoice > 2 {
					m.modalChoice = 0
				}
				return m, nil
			case "enter":
				return m.submitModal()
			case "y":
				m.modalChoice = 0
				return m.submitModal()
			case "n":
				m.modalChoice = 1
				return m.submitModal()
			case "c":
				m.modalChoice = 2
				return m.submitModal()
			case "esc", "q":
				m.modalOpen = false
				return m, nil
			}
			return m, nil
		}
		if m.running {
			switch msg.String() {
			case "ctrl+c", "esc", "q":
				return m, tea.Interrupt
			}
			return m, nil
		}
		switch msg.String() {
		case "R", "a":
			// Run every hook (full scaffold) via the modal.
			m.modalOpen = true
			return m, nil
		case "ctrl+c":
			return m, m.quitCmd()
		case "esc", "q":
			return m, m.quitCmd()
		case "left", "right", "tab":
			if m.focus == "list" {
				m.focus = "view"
			} else {
				m.focus = "list"
				m.viewport.SetContent(wrapForView(m.selectedHookContent(m.list.Index()), m.viewport.Width()))
				m.viewport.GotoTop()
			}
			return m, nil
		}
		if m.focus == "view" {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		if !m.running && !m.modalOpen && m.focus == "list" {
			m.viewport.SetContent(wrapForView(m.selectedHookContent(m.list.Index()), m.viewport.Width()))
			m.viewport.GotoTop()
		}
		return m, cmd
	}
	return m, nil
}

// submitModal resolves the modal choice: Run, Skip, or dismiss.
func (m hooksModel) submitModal() (hooksModel, tea.Cmd) {
	m.modalOpen = false
	switch m.modalChoice {
	case 1:
		return m.startRun(true)
	case 2:
		return m, nil
	default:
		return m.startRun(false)
	}
}

// startRun executes the full scaffold, streaming hook output into the
// right pane. skip disables hook execution but still renders files.
func (m hooksModel) startRun(skip bool) (hooksModel, tea.Cmd) {
	m.running = true
	m.modalOpen = false
	m.didRun = true
	m.focus = "view"
	m.output = ""
	if skip {
		m.output = "Skipping hooks (declined).\n"
	} else {
		m.output = "Running hooks...\n\n"
	}
	m.viewport.SetContent(wrapForView(m.output, m.viewport.Width()))
	m.viewport.GotoBottom()

	ch := make(chan string, 64)
	doneCh := make(chan error, 1)
	m.stream = ch
	m.doneCh = doneCh

	go func() {
		opts := template.HookOptions{PrintCommands: true, Verbose: m.verbose}
		if skip {
			opts.NoHooks = true
		} else {
			opts.Output = channelHookOutput(ch)
			opts.StepStart = func(kind, cmd string) {
				ch <- hookStepHeader(kind, cmd)
			}
		}
		doneCh <- m.tpl.RenderToWithPost(m.ctx, m.dest, m.resolved, opts)
		close(ch)
	}()
	return m, m.listen()
}

func (m hooksModel) quitCmd() tea.Cmd {
	if m.didRun {
		return tea.Quit
	}
	return tea.Interrupt
}

// listen resumes reading streamed hook output until the run finishes.
func (m hooksModel) listen() tea.Cmd {
	return func() tea.Msg {
		line, ok := <-m.stream
		if !ok {
			var err error
			if m.doneCh != nil {
				err = <-m.doneCh
			}
			return runDoneMsg{err: err}
		}
		return runLineMsg{text: line}
	}
}

func (m hooksModel) appBoundaryView(text string) string {
	return lipgloss.PlaceHorizontal(
		m.width,
		lipgloss.Left,
		m.styles.HeaderText.Render(text),
		lipgloss.WithWhitespaceChars(theme.BoundaryChar),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(theme.Accent)),
	)
}

func (m hooksModel) appBoundaryViewFoot(text string) string {
	return lipgloss.PlaceHorizontal(
		m.width,
		lipgloss.Left,
		lipgloss.NewStyle().PaddingRight(1).Render(text),
		lipgloss.WithWhitespaceChars(theme.BoundaryChar),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(theme.Accent)),
	)
}

func (m hooksModel) resize(width, height int) hooksModel {
	m.width = width
	m.height = height
	listH := max(height-7, 1)
	m.list.SetSize(hooksListContentW, listH)
	viewW := hooksViewContentW(width)
	m.viewport = viewport.New(viewport.WithWidth(viewW), viewport.WithHeight(listH))
	if !m.running && m.output == "" {
		m.viewport.SetContent(wrapForView(m.selectedHookContent(m.list.Index()), m.viewport.Width()))
		m.viewport.GotoTop()
	} else {
		m.viewport.SetContent(wrapForView(m.output, m.viewport.Width()))
	}
	return m
}

func (m hooksModel) view() tea.View {
	s := m.styles
	title := gradientText("Spin  Hooks Review: "+m.name, theme.AccentAlt, theme.Accent)
	header := m.appBoundaryView(title)

	listStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(1, 0)
	if m.focus == "list" && !m.modalOpen {
		listStyle = listStyle.BorderForeground(theme.StatusError)
	}
	listBox := listStyle.
		Width(hooksListContentW + hooksListBorderW).
		Height(max(m.viewport.Height(), 1) + 2).
		Render(m.list.View())

	viewStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(1, 0)

	if m.focus == "view" && !m.modalOpen {
		viewStyle = viewStyle.BorderForeground(theme.StatusWarning)
	}
	viewBox := viewStyle.
		Width(max(m.viewport.Width(), hooksViewMinW) + hooksViewBorderW).
		Height(max(m.viewport.Height(), 1) + 3).
		Render(m.viewport.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, listBox, " ", viewBox)

	var footer string
	switch {
	case m.modalOpen:
		footer = "←/→ toggle • enter submit • y Run • n Skip • c Cancel"
	case m.running:
		footer = "running scaffold…  (q/esc quit)"
	case m.output != "":
		if m.didRun {
			footer = "press q to exit • R run all again"
		} else {
			footer = "press q to exit"
		}
	default:
		footer = hookHintShort
	}
	footerView := m.appBoundaryViewFoot(footer)

	inner := header + "\n" + body + "\n" + footerView
	if m.modalOpen {
		// Size the canvas to the rendered base rather than m.width:
		// base re-adds the frame that m.width excludes, and a wider
		// canvas would clip the view pane's right border under the
		// modal.
		base := s.Base.Render(inner)
		modal := m.modalBox()
		canvas := lipgloss.NewCanvas(lipgloss.Width(base), lipgloss.Height(base))
		cw, ch := canvas.Width(), canvas.Height()
		boxW := lipgloss.Width(modal)
		boxH := lipgloss.Height(modal)
		x := max((cw-boxW)/2, 0)
		y := max((ch-boxH)/2, 0)
		comp := lipgloss.NewCompositor(
			lipgloss.NewLayer(base),
			lipgloss.NewLayer(modal).X(x).Y(y).Z(1),
		)
		canvas.Compose(comp)
		v := tea.NewView(canvas.Render())
		v.AltScreen = true
		v.BackgroundColor = theme.ViewBg
		return v
	}
	v := tea.NewView(s.Base.Render(inner))
	v.AltScreen = true
	v.BackgroundColor = theme.ViewBg
	return v
}

// modalBox renders the Run/Skip/Cancel dialog content; the caller
// composites it centered over the hooks view.
func (m hooksModel) modalBox() string {
	var b strings.Builder
	name := m.tpl.Name
	if m.tpl.SpinToml != nil && m.tpl.SpinToml.Name != "" {
		name = m.tpl.SpinToml.Name
	}
	fmt.Fprintf(&b, "Run %q hooks?\n\n", name)
	fmt.Fprintf(&b, "Only run hooks from templates you trust.\n")

	runStyle := lipgloss.NewStyle()
	skipStyle := lipgloss.NewStyle()
	cancelStyle := lipgloss.NewStyle()
	switch m.modalChoice {
	case 0:
		runStyle = runStyle.Foreground(theme.StatusError).Bold(true)
	case 1:
		skipStyle = skipStyle.Foreground(theme.StatusWarning).Bold(true)
	default:
		cancelStyle = cancelStyle.Foreground(theme.TextDimmed).Bold(true)
	}
	fmt.Fprintf(&b, "\n  %s    %s    %s\n",
		runStyle.Render("Run"),
		skipStyle.Render("Skip"),
		cancelStyle.Render("Cancel"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Accent).
		Padding(1, 3)
	return boxStyle.Render(b.String())
}
