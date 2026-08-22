//go:build windows

package main

import "golang.org/x/sys/windows"

// enableUTF8Console switches the current console's output codepage to UTF-8
// (65001). Without this, a console still on a legacy codepage (437/1252,
// common on a fresh Windows install) mis-decodes the UTF-8 emoji somm prints
// (🔧, ✅, ⚠️, ★) into "??" or mojibake — somm always writes UTF-8 bytes, it
// never controlled how the terminal interpreted them. Best-effort: a failure
// here (e.g. stdout redirected to a file, no real console attached) is
// silently ignored — worst case is the same mojibake a user already sees
// today, never a fatal error.
func enableUTF8Console() {
	_ = windows.SetConsoleOutputCP(65001)
}
