package holegen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PresetsFileName is the basename of the persisted preset store. It lives in
// the user's home directory (not the working directory) so presets are found
// no matter where the binary is launched from (spec §10.1).
const PresetsFileName = ".holegen_presets.json"

// Presets maps a preset name to that preset's RAW entry strings, keyed by
// Field.Key. Raw strings — not parsed numbers — so a value the user typed as
// "1.125in" round-trips and still reads as inches when reloaded.
type Presets map[string]map[string]string

// PresetsPath returns the absolute path of the preset store, or "" if the OS
// won't tell us the home directory (in which case presets work in memory for
// the session but don't persist).
func PresetsPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, PresetsFileName)
}

// LoadPresets reads the preset store. Missing, unreadable, or corrupt files
// all yield an empty (non-nil) map rather than an error — a bad preset file
// must never keep the app from working (spec §10.2).
//
// Unmarshalling straight into Presets also discards any entry whose shape
// isn't name→object-of-strings, which is the silent-drop behaviour the spec
// asks for.
func LoadPresets() Presets {
	path := PresetsPath()
	if path == "" {
		return Presets{}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return Presets{}
	}
	var p Presets
	if err := json.Unmarshal(data, &p); err != nil || p == nil {
		return Presets{}
	}
	return p
}

// SavePresets writes the store back with 2-space indentation. Unlike
// LoadPresets this DOES return its error — callers surface it in a dialog
// (spec §10.3).
func SavePresets(p Presets) error {
	path := PresetsPath()
	if path == "" {
		return os.ErrNotExist
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Names returns the preset names sorted alphabetically, ready to populate a
// dropdown (spec §10.4).
func (p Presets) Names() []string {
	names := make([]string, 0, len(p))
	for name := range p {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Values snapshots the current raw entry strings for every field, which is
// exactly what a preset stores. Whitespace is trimmed per field.
func Values(values map[string]string) map[string]string {
	out := make(map[string]string, len(Fields))
	for _, f := range Fields {
		out[f.Key] = strings.TrimSpace(values[f.Key])
	}
	return out
}
