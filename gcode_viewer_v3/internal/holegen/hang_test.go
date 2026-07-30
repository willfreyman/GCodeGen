package holegen

import (
	"strings"
	"testing"
)

// The helical descent loop subtracts PitchPerTurn from currentZ until it
// passes -totalDepth. A pitch of zero never decreases it, and a negative
// pitch increases it — either way the loop never terminates and appends
// G03 lines until the process dies. Neither value is rejected anywhere in
// spec §11's validation rules, so both are reachable by typing.
//
// These run with the package -timeout; if the guard regresses they hang
// rather than fail, which is exactly why the guard has to exist.
func TestZeroPitchIsRejectedNotHung(t *testing.T) {
	p := defaultParams(t)
	p.PitchPerTurn = 0

	if _, _, _, err := GenerateGCode(p); err == nil {
		t.Fatal("pitch 0 must be rejected — otherwise the descent loop never terminates")
	}
}

func TestNegativePitchIsRejectedNotHung(t *testing.T) {
	p := defaultParams(t)
	p.PitchPerTurn = -1

	if _, _, _, err := GenerateGCode(p); err == nil {
		t.Fatal("negative pitch must be rejected — the descent loop runs away upward")
	}
}

// ReadParams is the gate the UI actually goes through, and it runs on every
// keystroke to refresh the live estimate. It has to reject the same values,
// or typing "0" into the pitch field hangs the app before Generate is ever
// clicked.
func TestReadParamsRejectsNonPositivePitch(t *testing.T) {
	for _, bad := range []string{"0", "-1", "0.0"} {
		values := DefaultValues()
		values["pitchPerTurn"] = bad
		if _, err := ReadParams(values); err == nil {
			t.Errorf("pitch %q: want an error, got none", bad)
		}
	}
}

// An absurd grid allocates one Hole per cell and then ~10 G-code lines per
// hole. Because the live estimate re-runs on every keystroke, typing a long
// number into the row count would otherwise try to allocate progressively
// larger slices until the process is killed.
func TestReadParamsRejectsAbsurdGrid(t *testing.T) {
	values := DefaultValues()
	values["rowCount"] = "999999"
	values["columnCount"] = "999999"

	_, err := ReadParams(values)
	if err == nil {
		t.Fatal("want an error for a 10^12-hole grid")
	}
	if !strings.Contains(err.Error(), "holes") {
		t.Errorf("error should mention the hole count, got %q", err)
	}
}

// A large-but-plausible job must still be allowed.
func TestReadParamsAllowsLargeRealisticGrid(t *testing.T) {
	values := DefaultValues()
	values["rowCount"] = "200"
	values["columnCount"] = "40" // 8000 holes — big, but a real sheet job
	if _, err := ReadParams(values); err != nil {
		t.Errorf("8000 holes should be allowed, got %v", err)
	}
}
