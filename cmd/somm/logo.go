package main

import "fmt"

// sommLogo is a small wine-glass silhouette shown once, at the top of
// `somm setup`'s opening banner (see printBanner below) — plain ASCII only
// (no Unicode), on purpose: this prints before enableUTF8Console has any
// chance to matter and before we know anything about the host console's
// font/codepage, so it must render correctly everywhere without exception.
const sommLogo = `   ___
  /   \
  \___/
    |
  __|__
 /_____\`

// printBanner prints the startup logo + tagline once, before the
// interactive wizard takes over. Called from runSetup, right where the
// plain "Somm Setup Wizard" title used to stand alone.
func printBanner() {
	fmt.Println(titleStyle.Render(sommLogo))
	fmt.Println()
	fmt.Println(titleStyle.Render("Somm Setup Wizard") + " " + detailStyle.Render(version))
	fmt.Println(detailStyle.Render("Model Advisor for your AI agents"))
	fmt.Println()
}
