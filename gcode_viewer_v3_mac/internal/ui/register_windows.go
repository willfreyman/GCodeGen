//go:build windows

package ui

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// progID is the ProgID we register under HKCU\Software\Classes. Pointing
// every extension at the same ProgID means the open command + icon are
// defined once and shared.
const progID = "Nightbots.GcodeFile"

// gcodeExtensions matches the file picker filter and the macOS
// Info.plist UTI extensions.
var gcodeExtensions = []string{
	".nc", ".gcode", ".ngc", ".tap", ".cnc", ".gco",
	".g", ".mpf", ".nci", ".tab", ".eia", ".dnc",
}

// RegisterFileTypes writes the per-user registry entries that make
// double-clicking any common G-code file launch this exe with the file
// path as its first argument. Writes to HKCU only — no admin required,
// only affects the current user.
func RegisterFileTypes() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate own exe: %w", err)
	}

	if err := setKey(progID, "", "G-code program (GcodeSim)"); err != nil {
		return err
	}
	if err := setKey(progID+`\DefaultIcon`, "", fmt.Sprintf(`"%s",0`, exePath)); err != nil {
		return err
	}
	if err := setKey(progID+`\shell\open\command`, "", fmt.Sprintf(`"%s" "%%1"`, exePath)); err != nil {
		return err
	}
	for _, ext := range gcodeExtensions {
		if err := setKey(ext, "", progID); err != nil {
			return err
		}
	}

	notifyAssocChanged()
	return nil
}

// UnregisterFileTypes removes the registry entries created by Register.
// Only removes extension associations that STILL point at our ProgID.
func UnregisterFileTypes() error {
	classes, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes`, registry.WRITE)
	if err != nil {
		return fmt.Errorf("open HKCU\\Software\\Classes: %w", err)
	}
	defer classes.Close()

	for _, sub := range []string{
		progID + `\shell\open\command`,
		progID + `\shell\open`,
		progID + `\shell`,
		progID + `\DefaultIcon`,
		progID,
	} {
		_ = registry.DeleteKey(classes, sub)
	}

	for _, ext := range gcodeExtensions {
		k, err := registry.OpenKey(classes, ext, registry.READ)
		if err != nil {
			continue
		}
		cur, _, _ := k.GetStringValue("")
		k.Close()
		if cur == progID {
			_ = registry.DeleteKey(classes, ext)
		}
	}

	notifyAssocChanged()
	return nil
}

func setKey(sub, name, value string) error {
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Classes\`+sub,
		registry.WRITE,
	)
	if err != nil {
		return fmt.Errorf("create %s: %w", sub, err)
	}
	defer k.Close()
	if err := k.SetStringValue(name, value); err != nil {
		return fmt.Errorf("set %s value: %w", sub, err)
	}
	return nil
}

func notifyAssocChanged() {
	const (
		SHCNE_ASSOCCHANGED = 0x08000000
		SHCNF_IDLIST       = 0x0
	)
	shell32 := syscall.NewLazyDLL("shell32.dll")
	proc := shell32.NewProc("SHChangeNotify")
	_, _, _ = proc.Call(SHCNE_ASSOCCHANGED, SHCNF_IDLIST, 0, 0)
}
