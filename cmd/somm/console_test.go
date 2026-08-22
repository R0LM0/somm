package main

import "testing"

// TestEnableUTF8Console_DoesNotPanic runs on every platform this repo builds
// for: on Windows it exercises the real SetConsoleOutputCP call (best-effort,
// errors ignored by design — see console_windows.go), everywhere else it
// exercises the no-op in console_other.go. Either way, main() must never
// crash on this call before it has produced any output.
func TestEnableUTF8Console_DoesNotPanic(t *testing.T) {
	enableUTF8Console()
}
