package ui

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/sqweek/dialog"
)

// promptUpdateAvailable shows a native yes/no dialog asking the user if
// they want to open the GitHub Releases page for the newer version. Blocks
// until the user clicks (called from the main loop, briefly pauses
// rendering — same pattern as the file-open dialog). Returns true if the
// user clicked Yes.
func promptUpdateAvailable(current, latest string) bool {
	return dialog.Message(
		"%s is available.\n\nYou are running %s.\n\nOpen the download page in your browser?",
		displayVersion(latest), displayVersion(current),
	).Title("GcodeSim — Update Available").YesNo()
}

// promptHideForVersion is the follow-up dialog that fires only after the
// user declines the main update prompt. Lets them say "I don't want to
// be re-asked about this specific release" without permanently silencing
// future updates. Returns true if user wants to skip this version.
//
// Native dialog libraries that ship with Go bindings (sqweek/dialog)
// don't expose 3-button modals, so this is the cleanest portable way to
// add the third option without writing per-platform cgo for NSAlert /
// MessageBoxIndirect.
func promptHideForVersion(latest string) bool {
	return dialog.Message(
		"Hide the %s update notice until a newer version is released?\n\n(You can still see the title bar hint either way.)",
		displayVersion(latest),
	).Title("GcodeSim — Hide This Update?").YesNo()
}

// openReleasesPage opens the user's default browser to the specific
// release tag's page on GitHub. Cross-platform: uses `start` on Windows,
// `open` on macOS, `xdg-open` on Linux.
//
// Failure here is silently swallowed — if the user's system has no
// default browser configured we don't have a useful fallback, and
// the title bar still shows the update notice.
func openReleasesPage(tag string) {
	url := "https://github.com/willfreyman/GCodeGen/releases/tag/" + tag
	openInBrowser(url)
}

// openInBrowser launches the user's default web browser pointed at the
// given URL. Returns nil even on failure (browser may not be configured).
func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// `cmd /c start "" <url>` — the empty quoted string is required
		// because start treats the first quoted arg as the window title,
		// which would consume our URL otherwise.
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
