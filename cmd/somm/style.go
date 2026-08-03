package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	colorAccent  = lipgloss.Color("183")
	colorMuted   = lipgloss.Color("243")
	colorSuccess = lipgloss.Color("42")
	colorWarn    = lipgloss.Color("220")
	colorError   = lipgloss.Color("204")

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccent).
			Padding(1, 2)

	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	selectedItemStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	itemStyle         = lipgloss.NewStyle()
	helpStyle         = lipgloss.NewStyle().Foreground(colorMuted)
	successStyle      = lipgloss.NewStyle().Foreground(colorSuccess)
	warnStyle         = lipgloss.NewStyle().Foreground(colorWarn)
	errorStyle        = lipgloss.NewStyle().Foreground(colorError)
	// detailStyle sets off an inline path or version number from the
	// surrounding sentence, e.g. "✓ .env guardado en <path>".
	detailStyle = lipgloss.NewStyle().Italic(true).Foreground(colorMuted)
)

// renderScreen wraps a title, body and footer help line in the bordered
// layout shared by every somm TUI screen.
func renderScreen(title, body, help string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString(titleStyle.Render(title))
		b.WriteString("\n\n")
	}
	b.WriteString(body)
	if help != "" {
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render(help))
	}
	return boxStyle.Render(b.String())
}

// menuItem renders one menu/checklist row with a ► cursor when selected.
func menuItem(label string, selected bool) string {
	if selected {
		return selectedItemStyle.Render("► " + label)
	}
	return itemStyle.Render("  " + label)
}

const (
	helpNavigate = "j/k: navigate • enter: select • q: quit"
	helpConfirm  = "enter: confirm • esc: back • q: quit"
	helpContinue = "enter: continue • esc: back • q: quit"
)
