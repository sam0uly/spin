package cmd

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/sam0uly/spin/internal/params"
	"github.com/sam0uly/spin/internal/template"
	"github.com/sam0uly/spin/internal/theme"
)

type tuiStyles struct {
	Base, HeaderText, ErrorHeaderText, Status, StatusHeader lipgloss.Style
}

func newTUIStyles() *tuiStyles {
	return &tuiStyles{
		Base: lipgloss.NewStyle().Padding(1, 4, 0, 1),
		HeaderText: lipgloss.NewStyle().
			Foreground(theme.Accent).Bold(true).Padding(0, 1, 0, 2),
		ErrorHeaderText: lipgloss.NewStyle().
			Foreground(theme.ErrorFg).Bold(true).Padding(0, 1, 0, 2),
		Status: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(theme.Accent).
			PaddingLeft(1).MarginTop(1),
		StatusHeader: lipgloss.NewStyle().
			Foreground(theme.Accent).Bold(true),
	}
}

type tuiStep int

const (
	stepForm tuiStep = iota
	stepHooks
)

type newTUIModel struct {
	styles  *tuiStyles
	form    *huh.Form
	preview *viewport.Model
	width   int
	height  int
	tpl     *template.Template
	params  []params.Param
	step    tuiStep
	hooks   hooksModel

	ctx     context.Context
	dest    string
	name    string
	noHooks bool
	yes     bool
	verbose bool

	resolved map[string]any
}

func newNewTUIModel(tpl *template.Template, values map[string]any) (newTUIModel, error) {
	m := newTUIModel{tpl: tpl, styles: newTUIStyles(), width: termWidth(), height: 24}
	ps, err := params.Parse(tpl.SpinToml.Params)
	if err != nil {
		return m, err
	}
	params.SetDefaults(ps, values)
	// Seed supplied values onto matching params so the form opens
	// pre-filled, like the non-TTY path. An empty string must never
	// clobber a param's own default.
	for _, p := range ps {
		if v, ok := values[p.Name()]; ok {
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			p.Apply(params.FromAny(v))
		}
	}
	m.params = ps
	var fields []huh.Field
	for _, p := range ps {
		fields = append(fields, p.HuhField(values))
	}
	m.form = huh.NewForm(huh.NewGroup(fields...)).
		WithTheme(huh.ThemeFunc(func(_ bool) *huh.Styles { return theme.Theme() })).
		WithWidth(min(m.width/2, 60)).
		WithShowHelp(false).
		WithShowErrors(false)
	vp := viewport.New(viewport.WithWidth(46), viewport.WithHeight(10))
	vp.SoftWrap = true
	m.preview = &vp
	return m, nil
}

func (m newTUIModel) Init() tea.Cmd {
	return m.form.Init()
}

func (m newTUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = min(msg.Width, termWidth()) - m.styles.Base.GetHorizontalFrameSize()
		m.height = msg.Height
		if m.step == stepHooks {
			m.hooks = m.hooks.resize(m.width, m.height)
			return m, nil
		}
		m.form = m.form.WithWidth(min(m.width/2, 60)).WithHeight(msg.Height - 8)
		vp := viewport.New(viewport.WithWidth(46), viewport.WithHeight(max(msg.Height-13, 10)))
		vp.SoftWrap = true
		m.preview = &vp
		return m, nil
	case tea.KeyPressMsg:
		if m.step == stepHooks {
			var cmd tea.Cmd
			m.hooks, cmd = m.hooks.update(msg)
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Interrupt
		}
	}
	if m.step == stepHooks {
		var cmd tea.Cmd
		m.hooks, cmd = m.hooks.update(msg)
		return m, cmd
	}
	return m.updateForm(msg)
}

// updateForm runs the Update loop for the param form step. Ctrl+arrow
// scrolls the live preview.
func (m newTUIModel) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "ctrl+up", "ctrl+k":
			m.preview.PageUp()
			return m, nil
		case "ctrl+down", "ctrl+j":
			m.preview.PageDown()
			return m, nil
		}
	}
	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}
	if m.form.State == huh.StateCompleted {
		resolved := collectResolved(m.params, m.name)
		m.resolved = resolved
		if len(template.CollectHooks(m.tpl)) > 0 {
			m.step = stepHooks
			m.hooks = newHooksModel(m.tpl, m.styles, m.width, m.height, resolved, m.ctx, m.dest, m.name, m.noHooks, m.yes, m.verbose)
			return m, nil
		}
		return m, tea.Quit
	}
	return m, cmd
}

func (m newTUIModel) View() tea.View {
	switch m.step {
	case stepHooks:
		return m.hooks.view()
	default:
		return m.formView()
	}
}

func (m newTUIModel) formView() tea.View {
	s := m.styles

	title := gradientText("Spin  Create Project: "+m.tpl.Name, theme.AccentAlt, theme.Accent)

	v := strings.TrimSuffix(m.form.View(), "\n\n")
	form := lipgloss.NewStyle().Margin(1, 0).Render(v)

	status := m.statusView(form)

	errors := m.form.Errors()
	header := m.appBoundaryView(title)
	if len(errors) > 0 {
		header = m.appErrorBoundaryView(errorView(errors))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Left, form, status)
	footer := m.appBoundaryViewFoot(m.form.WithWidth(m.width-10).Help().ShortHelpView(m.form.KeyBinds()) + lipgloss.NewStyle().Foreground(theme.TextDimmed).Render(" • ") + lipgloss.NewStyle().Foreground(theme.Accent).Render("ctrl+↑/↓") + lipgloss.NewStyle().Foreground(theme.TextMuted).Render(" scroll preview"))
	m.form = m.form.WithWidth(min(m.width/2, 60))
	if len(errors) > 0 {
		footer = m.appErrorBoundaryView("")
	}
	hv := tea.NewView(s.Base.Render(header + "\n" + body + "\n\n" + footer))
	hv.AltScreen = true
	hv.BackgroundColor = theme.ViewBg
	return hv
}

func (m newTUIModel) statusView(form string) string {
	s := m.styles
	fw := lipgloss.Width(form)
	if fw > 0 && m.width-fw < 45 {
		return ""
	}
	w := max(m.width-fw-4, 50)
	label := lipgloss.NewStyle().Foreground(theme.Accent)
	cmdStyle := lipgloss.NewStyle().Foreground(theme.TextDimmed).Italic(true)
	dim := lipgloss.NewStyle().Foreground(theme.TextDimmed)
	thdr := func(text string, c color.Color) string {
		return lipgloss.NewStyle().Foreground(c).Bold(true).Render(text)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", thdr("Template", theme.StatusInfo))
	fmt.Fprintf(&b, "%s\n", m.tpl.Name)
	if m.tpl.SpinToml != nil {
		if m.tpl.SpinToml.Description != "" {
			fmt.Fprintf(&b, "%s\n", m.tpl.SpinToml.Description)
		}
		if m.tpl.SpinToml.Language != "" {
			fmt.Fprintf(&b, "Language: %s\n", m.tpl.SpinToml.Language)
		}
		if m.tpl.SpinToml.Type != "" {
			fmt.Fprintf(&b, "Type: %s\n", m.tpl.SpinToml.Type)
		}
		if len(m.tpl.SpinToml.Pre)+len(m.tpl.SpinToml.Post) > 0 {
			fmt.Fprintf(&b, "\n%s\n", thdr("Hooks", theme.StatusError))
			for _, pre := range m.tpl.SpinToml.Pre {
				fmt.Fprintf(&b, "  %s %s\n", label.Render("pre:"), cmdStyle.Render(pre.Run))
			}
			for _, post := range m.tpl.SpinToml.Post {
				fmt.Fprintf(&b, "  %s %s\n", label.Render("post:"), cmdStyle.Render(post.Run))
			}
		}
		if d := m.tpl.PreHookDir; d != "" {
			if es, _ := os.ReadDir(d); len(es) > 0 {
				fmt.Fprintf(&b, "\n%s\n", thdr("_pre/ scripts", theme.StatusError))
				for _, e := range es {
					if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
						fmt.Fprintf(&b, "  %s\n", dim.Render(e.Name()))
					}
				}
			}
		}
		if d := m.tpl.PostHookDir; d != "" {
			if es, _ := os.ReadDir(d); len(es) > 0 {
				fmt.Fprintf(&b, "\n%s\n", thdr("_post/ scripts", theme.StatusError))
				for _, e := range es {
					if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
						fmt.Fprintf(&b, "  %s\n", dim.Render(e.Name()))
					}
				}
			}
		}
	}
	fmt.Fprintf(&b, "\n%s\n", thdr("Params", theme.StatusWarning))
	for _, p := range m.params {
		if val := paramDisplay(p); val != "" {
			fmt.Fprintf(&b, "  %s: %s\n", p.Name(), val)
		}
	}
	fh := lipgloss.Height(form)
	vpH := max(fh-3, 1)
	if m.preview.Width() != w-4 || m.preview.Height() != vpH {
		vp := viewport.New(viewport.WithWidth(w-4), viewport.WithHeight(vpH))
		vp.SoftWrap = true
		*m.preview = vp
	}
	m.preview.SetContent(b.String())
	ml := max(m.width-w-fw-s.Status.GetMarginRight(), 0)
	return s.Status.Width(w).Height(lipgloss.Height(form)).MarginLeft(ml).Render(m.preview.View())
}

func errorView(errs []error) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString(e.Error())
	}
	return b.String()
}

func paramDisplay(p params.Param) string {
	v := p.Value()
	switch v.Kind {
	case params.TypeText, params.TypeTextarea, params.TypeSelect, params.TypeSecret, params.TypeLicense:
		return v.String
	case params.TypeNumber:
		return fmt.Sprintf("%d", v.Int)
	case params.TypeBool:
		if v.Bool {
			return "Yes"
		}
		return "No"
	case params.TypeMultiSelect:
		return strings.Join(v.List, ", ")
	case params.TypePath:
		return v.Path
	}
	return ""
}

func gradientText(s string, from, to color.Color) string {
	if len(s) == 0 {
		return ""
	}
	colors := lipgloss.Blend1D(len(s), from, to)
	var b strings.Builder
	for i, r := range s {
		b.WriteString(lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(string(r)))
	}
	return b.String()
}

func (m newTUIModel) appBoundaryView(text string) string {
	return lipgloss.PlaceHorizontal(
		m.width,
		lipgloss.Left,
		m.styles.HeaderText.Render(text),
		lipgloss.WithWhitespaceChars(theme.BoundaryChar),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(theme.Accent)),
	)
}

func (m newTUIModel) appErrorBoundaryView(text string) string {
	return lipgloss.PlaceHorizontal(
		m.width,
		lipgloss.Left,
		m.styles.ErrorHeaderText.Render(text),
		lipgloss.WithWhitespaceChars(theme.BoundaryChar),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(theme.StatusError)),
	)
}

func (m newTUIModel) appBoundaryViewFoot(text string) string {
	return lipgloss.PlaceHorizontal(
		m.width,
		lipgloss.Left,
		lipgloss.NewStyle().PaddingRight(1).Render(text),
		lipgloss.WithWhitespaceChars(theme.BoundaryChar),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Foreground(theme.Accent)),
	)
}

// collectResolved merges the answered params with the implicit
// name/project_name keys every template can reference.
func collectResolved(ps []params.Param, name string) map[string]any {
	out := map[string]any{
		"name":         name,
		"project_name": name,
	}
	for _, p := range ps {
		out[p.Name()] = template.UnwrapValue(p.Value())
	}
	return out
}

// runNewTUI runs the interactive form, and (when the template has
// hooks) the hook review screen. It returns whether the scaffold was
// already executed by the TUI (so runNew can skip its own render), the
// resolved param values, and any error.
func runNewTUI(tpl *template.Template, values map[string]any, ctx context.Context, dest, name string, noHooks, yes, verbose bool) (bool, map[string]any, error) {
	m, err := newNewTUIModel(tpl, values)
	if err != nil {
		return false, nil, err
	}
	m.ctx = ctx
	m.dest = dest
	m.name = name
	m.noHooks = noHooks
	m.yes = yes
	m.verbose = verbose
	p := tea.NewProgram(m)
	final, err := p.Run()
	if err != nil {
		return false, nil, err
	}
	fm := final.(newTUIModel)
	if fm.resolved == nil {
		fm.resolved = collectResolved(fm.params, fm.name)
	}
	return fm.hooks.didRun, fm.resolved, nil
}
