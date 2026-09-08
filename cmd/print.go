package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"golang.org/x/term"

	"samouly.fun/spin/internal/log"
	"samouly.fun/spin/internal/theme"
)

var (
	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(theme.Accent)
	styleCell   = lipgloss.NewStyle()
	styleBorder = lipgloss.NewStyle().Foreground(theme.TextDimmed)
)

func printSuccess(format string, args ...any) {
	log.Stdout.Info(fmt.Sprintf(format, args...))
}
func printInfo(format string, args ...any) {
	log.Stdout.Info(fmt.Sprintf(format, args...))
}
func printWarn(format string, args ...any) {
	log.Warn(fmt.Sprintf(format, args...))
}
func printHint(format string, args ...any) {
	style := lipgloss.NewStyle().
		MarginTop(1).
		Foreground(theme.TextDimmed).
		Italic(true)
	fmt.Fprintln(os.Stdout, style.Render(fmt.Sprintf(format, args...)))
}

func printTable(w io.Writer, headers []string, rows [][]string) {
	if len(rows) == 0 {
		return
	}
	pad := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
	headerStyle := pad.Inherit(styleHeader)
	cellStyle := pad.Inherit(styleCell)
	t := table.New().
		Headers(headers...).
		Rows(rows...).
		Width(termWidth()).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(true).
		BorderRow(true).
		BorderStyle(styleBorder).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		})
	_, _ = fmt.Fprintln(w, t.Render())
}

// termWidth returns the terminal width from the TTY, $COLUMNS, or 120.
func termWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	if s := os.Getenv("COLUMNS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return 120
}

func shrinkCol(headers []string, rows [][]string) [][]string {
	if len(rows) == 0 || len(headers) == 0 {
		return rows
	}
	termW := termWidth()
	overhead := (len(headers) - 1) + (len(headers) * 2)
	fixedW := 0
	for i := 0; i < len(headers)-1; i++ {
		w := len(headers[i])
		for _, r := range rows {
			if len(r[i]) > w {
				w = len(r[i])
			}
		}
		fixedW += w
	}
	flexW := max(termW-overhead-fixedW, 10)
	out := make([][]string, len(rows))
	last := len(headers) - 1
	for i, r := range rows {
		cp := make([]string, len(r))
		copy(cp, r)
		cp[last] = truncate(r[last], flexW)
		out[i] = cp
	}
	return out
}

func truncate(s string, n int) string {
	if n <= 1 || len(s) <= n {
		return s
	}
	return strings.TrimRight(s[:n-1], " ") + "…"
}
