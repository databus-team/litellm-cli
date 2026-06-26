package testutils

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// AnsiPattern is a comprehensive regular expression to strip CSI and OSC escape sequences.
const AnsiPattern = "\x1b\\[[0-9;]*[a-zA-Z]|\x1b]8;;[^\x1b\x07]*(?:\x1b\\\\|\x07)"

var ansiRegexp = regexp.MustCompile(AnsiPattern)

// StripANSI removes all ANSI escape sequences (colors, styles, hyperlinks, etc.) from the string.
func StripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

// T is an interface matching the subset of *testing.T methods used by AssertTUISnapshot.
type T interface {
	Helper()
	Fatalf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Logf(format string, args ...interface{})
}

// AssertTUISnapshot asserts that the actual terminal output matches the stored golden snapshot.
// It manages two files under "testdata/snapshots/":
// 1. <snapshotName>.ansi.snap - contains the raw output with ANSI color and style sequences.
// 2. <snapshotName>.txt.snap - contains the plain text with all ANSI sequences stripped.
func AssertTUISnapshot(t T, snapshotName string, actual string) {
	t.Helper()

	// Normalize newlines to Unix style
	actual = strings.ReplaceAll(actual, "\r\n", "\n")
	actualTxt := StripANSI(actual)

	snapshotDir := filepath.Join("testdata", "snapshots")
	ansiPath := filepath.Join(snapshotDir, snapshotName+".ansi.snap")
	txtPath := filepath.Join(snapshotDir, snapshotName+".txt.snap")

	update := os.Getenv("UPDATE_SNAPSHOTS") == "true" || os.Getenv("UPDATE_GOLDEN") == "true"

	if update {
		// Create the snapshots directory if it does not exist
		if err := os.MkdirAll(snapshotDir, 0755); err != nil {
			t.Fatalf("failed to create snapshot directory %s: %v", snapshotDir, err)
		}

		// Write raw ANSI snapshot
		if err := os.WriteFile(ansiPath, []byte(actual), 0644); err != nil {
			t.Fatalf("failed to write ANSI snapshot to %s: %v", ansiPath, err)
		}

		// Write stripped plain text snapshot
		if err := os.WriteFile(txtPath, []byte(actualTxt), 0644); err != nil {
			t.Fatalf("failed to write TXT snapshot to %s: %v", txtPath, err)
		}

		t.Logf("Successfully updated snapshots for %s", snapshotName)
		return
	}

	// Read and verify ANSI snapshot
	ansiBytes, err := os.ReadFile(ansiPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("ANSI snapshot file %s does not exist. Run with UPDATE_SNAPSHOTS=true to record it.", ansiPath)
		}
		t.Fatalf("failed to read ANSI snapshot %s: %v", ansiPath, err)
	}
	expectedAnsi := strings.ReplaceAll(string(ansiBytes), "\r\n", "\n")

	// Read and verify TXT snapshot
	txtBytes, err := os.ReadFile(txtPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("TXT snapshot file %s does not exist. Run with UPDATE_SNAPSHOTS=true to record it.", txtPath)
		}
		t.Fatalf("failed to read TXT snapshot %s: %v", txtPath, err)
	}
	expectedTxt := strings.ReplaceAll(string(txtBytes), "\r\n", "\n")

	// Verify TXT snapshot first (content is primary)
	if actualTxt != expectedTxt {
		diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
			A:        difflib.SplitLines(expectedTxt),
			B:        difflib.SplitLines(actualTxt),
			FromFile: "Expected (TXT)",
			ToFile:   "Actual (TXT)",
			Context:  3,
		})
		t.Errorf("TXT snapshot mismatch for %s:\n%s\nHint: If this change is intentional, run 'UPDATE_SNAPSHOTS=true go test ./...' to update snapshots.", snapshotName, diff)
		return // Return early since content mismatch implies visual mismatch
	}

	// Verify ANSI snapshot second (style/color is secondary)
	if actual != expectedAnsi {
		diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
			A:        difflib.SplitLines(expectedAnsi),
			B:        difflib.SplitLines(actual),
			FromFile: "Expected (ANSI)",
			ToFile:   "Actual (ANSI)",
			Context:  3,
		})
		t.Errorf("ANSI snapshot mismatch for %s:\n%s\nHint: If this change is intentional, run 'UPDATE_SNAPSHOTS=true go test ./...' to update snapshots.", snapshotName, diff)
	}
}
