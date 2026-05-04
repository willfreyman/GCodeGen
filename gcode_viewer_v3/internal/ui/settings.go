package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings is the small JSON blob persisted to the user's config dir
// across launches. Currently just remembers which release versions the
// user has dismissed via "don't ask again". Easy to extend with future
// preferences (last bit diameter, theme, etc.) — just add fields and
// they'll round-trip through Load/Save.
type Settings struct {
	SkippedVersions []string `json:"skipped_versions,omitempty"`
}

// settingsPath returns the absolute path to the user's settings file.
// Cross-platform via os.UserConfigDir():
//   * Windows: %AppData%\GcodeSim\settings.json
//   * macOS:   ~/Library/Application Support/GcodeSim/settings.json
//   * Linux:   ~/.config/GcodeSim/settings.json
//
// Returns "" if the OS doesn't expose a config dir (rare edge case);
// caller treats that as "no persistence" — settings still work in-memory
// but don't survive a restart.
func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "GcodeSim", "settings.json")
}

// LoadSettings reads the persisted JSON, returning a zero-valued Settings
// if the file is missing, empty, or corrupted (we never want a bad
// settings file to keep the app from launching).
func LoadSettings() *Settings {
	path := settingsPath()
	if path == "" {
		return &Settings{}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return &Settings{}
	}
	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		// Corrupt JSON — drop it on the floor and start fresh. Better
		// than crashing or refusing to launch.
		return &Settings{}
	}
	return &s
}

// Save writes the settings to disk, creating the parent dir if needed.
// Errors are silently swallowed — if we can't write (read-only fs, full
// disk, etc.) we still want the app to keep running with in-memory state.
func (s *Settings) Save() {
	path := settingsPath()
	if path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// IsVersionSkipped reports whether the user has previously dismissed the
// update prompt for this exact version tag.
func (s *Settings) IsVersionSkipped(version string) bool {
	for _, v := range s.SkippedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// SkipVersion adds a version to the dismissed list (no-op if already
// present) and persists.
func (s *Settings) SkipVersion(version string) {
	if s.IsVersionSkipped(version) {
		return
	}
	s.SkippedVersions = append(s.SkippedVersions, version)
	s.Save()
}
