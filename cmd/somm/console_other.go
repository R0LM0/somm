//go:build !windows

package main

// enableUTF8Console is a no-op outside Windows: every other target platform
// somm ships for (darwin, linux) already defaults its terminal to UTF-8, so
// there is no console codepage to switch. See console_windows.go for why
// this exists at all.
func enableUTF8Console() {}
