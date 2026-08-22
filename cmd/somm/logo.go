package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// renderLogo renders sommLogoLines with sommLogoColors' true color applied
// per Braille cell (sampled straight from img/grapes.png's own pixels — see
// logo_data.go), instead of gentle-ai's fixed row-gradient. Cells with no
// color (the transparent parts of the source image, "") pass through
// unstyled — they're the blank Braille char (U+2800) anyway, so it never
// matters visually. lipgloss already no-ops color codes outside a real TTY,
// so this degrades to plain (correctly shaped) Braille art automatically.
//
// Row/column bounds are checked rather than assumed: sommLogoLines and
// sommLogoColors are hand-copied generated data (see tools/img2braille), so
// a transcription slip that drops or duplicates a row must degrade to a
// plain, uncolored line instead of an index-out-of-range panic on startup.
func renderLogo() string {
	var out string
	for i, line := range sommLogoLines {
		var colors []string
		if i < len(sommLogoColors) {
			colors = sommLogoColors[i]
		}
		runes := []rune(line)
		for j, r := range runes {
			if j < len(colors) && colors[j] != "" {
				out += lipgloss.NewStyle().Foreground(lipgloss.Color(colors[j])).Render(string(r))
			} else {
				out += string(r)
			}
		}
		if i < len(sommLogoLines)-1 {
			out += "\n"
		}
	}
	return out
}

// printBanner prints the startup logo + tagline once, before the
// interactive wizard takes over. Called from runSetup, right where the
// plain "Somm Setup Wizard" title used to stand alone.
func printBanner() {
	fmt.Println(renderLogo())
	fmt.Println()
	fmt.Println(titleStyle.Render("Somm Setup Wizard") + " " + detailStyle.Render(version))
	fmt.Println(detailStyle.Render("Model Advisor for your AI agents"))
	fmt.Println()
}
