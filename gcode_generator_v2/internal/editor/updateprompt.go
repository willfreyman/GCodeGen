package editor

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"gcodegen.local/generator/internal/shared"
	"gcodegen.local/generator/internal/version"

	"github.com/sqweek/dialog"
)

// checkForUpdates runs the version probe in a goroutine and, when a
// newer release is available and not previously skipped, pops the
// native yes/no prompt. Mirrors v3's pattern in internal/ui — same
// 5-second timeout in the version package, same skip-this-version flow.
//
// Skipped on dev builds (no meaningful "current" to compare).
func checkForUpdates() {
	if version.IsDev() {
		return
	}
	go func() {
		latest, err := version.LatestRelease()
		if err != nil || latest == "" || normalizedEqual(latest, version.Version) {
			return
		}
		settings := shared.LoadSettings()
		if settings.IsVersionSkipped(latest) {
			return
		}
		if promptUpdateAvailable(version.Version, latest) {
			openReleasesPage(latest)
			return
		}
		if promptHideForVersion(latest) {
			settings.SkipVersion(latest)
		}
	}()
}

func promptUpdateAvailable(current, latest string) bool {
	return dialog.Message(
		"%s is available.\n\nYou are running %s.\n\nOpen the download page in your browser?",
		displayVersion(latest), displayVersion(current),
	).Title("GcodeGen — Update Available").YesNo()
}

func promptHideForVersion(latest string) bool {
	return dialog.Message(
		"Hide the %s update notice until a newer version is released?",
		displayVersion(latest),
	).Title("GcodeGen — Hide This Update?").YesNo()
}

func openReleasesPage(tag string) {
	url := "https://github.com/willfreyman/GCodeGen/releases/tag/" + tag
	openInBrowser(url)
}

func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
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

// displayVersion normalizes "v3.0.2" → "V3.0.2"; "" or "dev" → "(dev)".
func displayVersion(v string) string {
	if v == "" || v == "dev" {
		return "(dev)"
	}
	return "V" + strings.TrimLeft(v, "vV")
}

// normalizedEqual reports whether two version strings represent the
// same release modulo a leading "v" / "V".
func normalizedEqual(a, b string) bool {
	return strings.TrimLeft(a, "vV") == strings.TrimLeft(b, "vV")
}
