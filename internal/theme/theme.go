// Package theme defines the spin color palette and style sets shared
// by prompts, TUIs, and CLI help output.
package theme

import (
	"image/color"
	"os"

	"charm.land/bubbles/v2/list"
	"charm.land/fang/v2"
	"charm.land/huh/v2"
	huhspinner "charm.land/huh/v2/spinner"
	"charm.land/lipgloss/v2"
)

var (
	isDark = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	ld     = lipgloss.LightDark(isDark)

	orange        = lipgloss.Color("#ff5a1f")
	orangeSoft    = lipgloss.Color("#fdba74")
	orangeVibrant = lipgloss.Color("#ff7a45")
	orangeDark    = lipgloss.Color("#e84a0a")

	warmGray           = lipgloss.Color("#a5a4a1")
	warmGraySoft       = lipgloss.Color("#dcd3c4")
	warmGrayLight      = lipgloss.Color("#e5ddd0")
	warmGrayLightest   = lipgloss.Color("#f2ede4")
	warmGraySuperLight = lipgloss.Color("#fbfaf7")
	warmGrayDim        = lipgloss.Color("#78716c")
	warmGrayTone       = lipgloss.Color("#57534e")
	warmGrayDark       = lipgloss.Color("#44403c")
	warmGrayDarker     = lipgloss.Color("#2c2a26")
	warmGrayDarkest    = lipgloss.Color("#0c0a09")

	Accent      = ld(orange, orangeVibrant)
	AccentAlt   = ld(orangeVibrant, orangeSoft)
	text        = ld(warmGrayDark, warmGrayLightest)
	TextMuted   = ld(warmGrayDim, warmGray)
	TextDimmed  = ld(warmGray, warmGrayDim)
	textSubdued = ld(warmGraySoft, warmGrayTone)

	StatusError   = ld(orangeDark, lipgloss.Color("#ED567A"))
	StatusSuccess = ld(lipgloss.Color("#15803d"), lipgloss.Color("#4ade80"))
	StatusInfo    = ld(lipgloss.Color("#1d4ed8"), lipgloss.Color("#60a5fa"))
	StatusWarning = orangeSoft

	ErrorFg = ld(orangeDark, lipgloss.Color("9"))
	ViewBg  = ld(warmGraySuperLight, warmGrayDarkest)

	BoundaryChar = "/"
)

// Theme returns the huh styles used by every interactive form.
func Theme() *huh.Styles {
	t := huh.ThemeBase(isDark)
	border := ld(orangeSoft, warmGrayTone)
	mutedBg := ld(warmGrayLight, warmGrayDark)

	t.Focused.Base = t.Focused.Base.BorderForeground(border)
	t.Focused.Card = t.Focused.Base
	t.Focused.Title = t.Focused.Title.Foreground(Accent).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(Accent).Bold(true).MarginBottom(1)
	t.Focused.Directory = t.Focused.Directory.Foreground(AccentAlt)
	t.Focused.File = t.Focused.File.Foreground(text)
	t.Focused.Description = t.Focused.Description.Foreground(TextMuted)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(StatusError)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(StatusError)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(Accent)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(AccentAlt)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(AccentAlt)
	t.Focused.Option = t.Focused.Option.Foreground(text)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(Accent)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(Accent)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(ld(orangeDark, orangeVibrant)).SetString("✓ ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(TextDimmed).SetString("• ")
	t.Focused.UnselectedOption = t.Focused.UnselectedOption.Foreground(text)
	t.Focused.FocusedButton = t.Focused.FocusedButton.
		Foreground(ld(warmGraySuperLight, warmGrayDarkest)).
		Background(Accent)
	t.Focused.BlurredButton = t.Focused.BlurredButton.
		Foreground(text).
		Background(mutedBg)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(Accent)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(TextDimmed)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(Accent)
	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()
	t.Help.Ellipsis = t.Help.Ellipsis.Foreground(TextMuted)
	t.Help.ShortKey = t.Help.ShortKey.Foreground(Accent)
	t.Help.ShortDesc = t.Help.ShortDesc.Foreground(TextMuted)
	t.Help.ShortSeparator = t.Help.ShortSeparator.Foreground(TextDimmed)
	t.Help.FullKey = t.Help.FullKey.Foreground(Accent)
	t.Help.FullDesc = t.Help.FullDesc.Foreground(TextMuted)
	t.Help.FullSeparator = t.Help.FullSeparator.Foreground(TextDimmed)
	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}

// SpinnerTheme returns the spinner styles; the dark flag is unused
// because colors adapt through LightDark.
func SpinnerTheme(_ bool) *huhspinner.Styles {
	return &huhspinner.Styles{
		Spinner: lipgloss.NewStyle().Foreground(Accent),
		Title:   lipgloss.NewStyle().Foreground(text),
	}
}

// NewForm wraps a huh form with the spin theme.
func NewForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).WithTheme(huh.ThemeFunc(func(_ bool) *huh.Styles {
		return Theme()
	}))
}

// FangColorScheme returns the color scheme for fang-rendered help
// output, resolved against fld for light/dark terminals.
func FangColorScheme(fld lipgloss.LightDarkFunc) fang.ColorScheme {
	return fang.ColorScheme{
		Base:           fld(warmGrayDark, warmGraySuperLight),
		Title:          orange,
		Description:    fld(warmGrayDim, warmGray),
		Codeblock:      fld(warmGrayLight, warmGrayDarker),
		Program:        orangeVibrant,
		Command:        orange,
		DimmedArgument: fld(warmGray, warmGrayDim),
		Comment:        fld(warmGrayDim, warmGray),
		Flag:           orangeVibrant,
		FlagDefault:    fld(warmGrayDim, warmGray),
		QuotedString:   fld(orangeDark, orangeSoft),
		Argument:       fld(warmGrayDark, warmGraySuperLight),
		Help:           fld(warmGrayDim, warmGray),
		Dash:           fld(warmGrayDim, warmGray),
		ErrorHeader:    [2]color.Color{warmGraySuperLight, StatusError},
		ErrorDetails:   StatusError,
	}
}

// ListStyles returns the bubbles list chrome styles.
func ListStyles() list.Styles {
	s := list.DefaultStyles(isDark)
	s.Title = lipgloss.NewStyle().
		Background(Accent).
		Foreground(ld(warmGraySuperLight, warmGrayDarkest)).
		Padding(0, 1)
	s.StatusBar = s.StatusBar.Foreground(TextMuted)
	s.ActivePaginationDot = s.ActivePaginationDot.Foreground(Accent)
	return s
}

// ListItemStyles returns the bubbles list item styles.
func ListItemStyles() list.DefaultItemStyles {
	s := list.NewDefaultItemStyles(isDark)
	s.NormalTitle = lipgloss.NewStyle().Foreground(text).Padding(0, 0, 0, 2)
	s.NormalDesc = s.NormalTitle.Foreground(TextMuted)
	s.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(Accent).
		Foreground(ld(orange, orangeSoft)).
		Padding(0, 0, 0, 1)
	s.SelectedDesc = s.SelectedTitle.Foreground(AccentAlt)
	s.DimmedTitle = lipgloss.NewStyle().Foreground(TextDimmed).Padding(0, 0, 0, 2)
	s.DimmedDesc = s.DimmedTitle.Foreground(textSubdued)
	return s
}
