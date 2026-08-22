package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// logoBrightenFactor pulls every sampled logo color toward white by this
// fraction (a "screen blend": c' = c + (255-c)*factor). img/logo.png is a
// dark-mode illustration whose colors are already fairly muted, and by the
// time a 2x4 pixel block gets downsampled/averaged into one Braille cell
// (see tools/img2braille), the result reads as too close to the terminal's
// own near-black background to stand out. This is a render-time fix, not a
// data regeneration: it applies uniformly to whatever image the logo data
// was last generated from, so retuning it doesn't require re-running the
// conversion pipeline.
const logoBrightenFactor = 0.42

// brightenHex lightens a "#rrggbb" color toward white by logoBrightenFactor.
// Malformed input (should never happen — sommLogoColors is either "" or a
// valid 6-digit hex from tools/img2braille) passes through unchanged rather
// than panicking or dropping the cell's color.
func brightenHex(hex string) string {
	if len(hex) != 7 || hex[0] != '#' {
		return hex
	}
	r, errR := strconv.ParseUint(hex[1:3], 16, 8)
	g, errG := strconv.ParseUint(hex[3:5], 16, 8)
	b, errB := strconv.ParseUint(hex[5:7], 16, 8)
	if errR != nil || errG != nil || errB != nil {
		return hex
	}
	lighten := func(c uint64) uint64 {
		return c + uint64(float64(255-c)*logoBrightenFactor)
	}
	var sb strings.Builder
	sb.WriteByte('#')
	fmt.Fprintf(&sb, "%02x%02x%02x", lighten(r), lighten(g), lighten(b))
	return sb.String()
}

// renderLogo renders sommLogoLines with sommLogoColors' true color applied
// per Braille cell (sampled straight from img/logo.png's own pixels — see
// logo_data.go), brightened via brightenHex, instead of gentle-ai's fixed
// row-gradient. Cells with no color (the transparent/background parts of
// the source image, "") pass through unstyled — they're the blank Braille
// char (U+2800) anyway, so it never matters visually. lipgloss already
// no-ops color codes outside a real TTY, so this degrades to plain
// (correctly shaped) Braille art automatically.
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
				out += lipgloss.NewStyle().Foreground(lipgloss.Color(brightenHex(colors[j]))).Render(string(r))
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
