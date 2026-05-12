// Package shared holds tiny utilities used by both the editor and its
// aux subprocesses. Mostly settings persistence + anything else that
// shouldn't depend on either Ebiten or g3n.
package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings is the small JSON blob persisted to the user's config dir
// across launches. Currently remembers dismissed update versions and
// the last-used material/bit values so the preview window starts
// where the user left it.
type Settings struct {
	SkippedVersions []string `json:"skipped_versions,omitempty"`
	LastMaterial    string   `json:"last_material,omitempty"`
	LastBitMM       float64  `json:"last_bit_mm,omitempty"`
}

// settingsPath returns the absolute path to the editor's settings file.
// Distinct from the viewer (GcodeSim) so the two apps don't clobber
// each other's preferences:
//   * Windows: %AppData%\GcodeGen\settings.json
//   * macOS:   ~/Library/Application Support/GcodeGen/settings.json
//   * Linux:   ~/.config/GcodeGen/settings.json
func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	return filepath.Join(dir, "GcodeGen", "settings.json")
}

// LoadSettings reads persisted JSON, returning zero-valued settings
// if the file is missing, empty, or corrupt — never want a bad config
// file to block startup.
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
		return &Settings{}
	}
	return &s
}

// Save writes settings to disk, creating the parent dir as needed.
// Errors are silently swallowed (read-only fs, full disk, etc.).
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

// IsVersionSkipped reports whether the user dismissed a specific
// update prompt.
func (s *Settings) IsVersionSkipped(version string) bool {
	for _, v := range s.SkippedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// SkipVersion adds a version to the dismissed list and persists.
func (s *Settings) SkipVersion(version string) {
	if s.IsVersionSkipped(version) {
		return
	}
	s.SkippedVersions = append(s.SkippedVersions, version)
	s.Save()
}
