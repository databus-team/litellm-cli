package testutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mockT is a helper to capture test results during our self-testing of AssertTUISnapshot.
type mockT struct {
	errors []string
	fatals []string
	logs   []string
}

func (m *mockT) Helper() {}

func (m *mockT) Errorf(format string, args ...interface{}) {
	m.errors = append(m.errors, fmt.Sprintf(format, args...))
}

func (m *mockT) Fatalf(format string, args ...interface{}) {
	m.fatals = append(m.fatals, fmt.Sprintf(format, args...))
}

func (m *mockT) Logf(format string, args ...interface{}) {
	m.logs = append(m.logs, fmt.Sprintf(format, args...))
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain text",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "standard red color",
			input:    "\x1b[31mhello\x1b[0m world",
			expected: "hello world",
		},
		{
			name:     "bold and underline lipgloss style",
			input:    "\x1b[1;4mstyles\x1b[0m",
			expected: "styles",
		},
		{
			name:     "true color rgb",
			input:    "\x1b[38;2;255;100;50mRGB Color\x1b[0m",
			expected: "RGB Color",
		},
		{
			name:     "osc8 hyperlink with ST (backslash)",
			input:    "\x1b]8;;http://example.com\x1b\\Click Here\x1b]8;;\x1b\\",
			expected: "Click Here",
		},
		{
			name:     "osc8 hyperlink with BEL (bell)",
			input:    "\x1b]8;;http://example.com\x07Click Here\x1b]8;;\x07",
			expected: "Click Here",
		},
		{
			name:     "complex mix of styles and newlines",
			input:    "\x1b[1mHeader\x1b[0m\n\x1b[38;5;12mLine 1\x1b[0m\n\x1b]8;;http://test.com\x07Link\x1b]8;;\x07",
			expected: "Header\nLine 1\nLink",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := StripANSI(tt.input)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestAssertTUISnapshot(t *testing.T) {
	snapshotName := "test_mock_tui_snapshot"
	snapshotDir := filepath.Join("testdata", "snapshots")
	ansiPath := filepath.Join(snapshotDir, snapshotName+".ansi.snap")
	txtPath := filepath.Join(snapshotDir, snapshotName+".txt.snap")

	// Ensure clean state before running test
	os.Remove(ansiPath)
	os.Remove(txtPath)
	defer func() {
		os.Remove(ansiPath)
		os.Remove(txtPath)
	}()

	// Save original env vars
	origUpdateSnapshots := os.Getenv("UPDATE_SNAPSHOTS")
	origUpdateGolden := os.Getenv("UPDATE_GOLDEN")
	defer func() {
		os.Setenv("UPDATE_SNAPSHOTS", origUpdateSnapshots)
		os.Setenv("UPDATE_GOLDEN", origUpdateGolden)
	}()

	// 1. Test when snapshot file does not exist (update=false)
	t.Run("missing snapshot files", func(t *testing.T) {
		os.Setenv("UPDATE_SNAPSHOTS", "false")
		os.Setenv("UPDATE_GOLDEN", "false")

		mt := &mockT{}
		AssertTUISnapshot(mt, snapshotName, "some content")

		if len(mt.fatals) == 0 {
			t.Fatal("expected fatal error due to missing snapshot files, but got none")
		}
		if !strings.Contains(mt.fatals[0], "does not exist") {
			t.Errorf("expected missing file error message, got: %s", mt.fatals[0])
		}
	})

	// 2. Test recording/updating snapshots (update=true)
	t.Run("record snapshots", func(t *testing.T) {
		os.Setenv("UPDATE_SNAPSHOTS", "true")

		mt := &mockT{}
		content := "\x1b[31mHello\x1b[0m\r\nWorld" // contains CRLF to test normalization
		AssertTUISnapshot(mt, snapshotName, content)

		if len(mt.errors) > 0 || len(mt.fatals) > 0 {
			t.Fatalf("unexpected error during recording: errors=%v, fatals=%v", mt.errors, mt.fatals)
		}

		// Verify files were created
		ansiBytes, err := os.ReadFile(ansiPath)
		if err != nil {
			t.Fatalf("failed to read created ANSI snapshot: %v", err)
		}
		txtBytes, err := os.ReadFile(txtPath)
		if err != nil {
			t.Fatalf("failed to read created TXT snapshot: %v", err)
		}

		// Verify content (CRLF should be normalized to LF)
		expectedAnsi := "\x1b[31mHello\x1b[0m\nWorld"
		expectedTxt := "Hello\nWorld"

		if string(ansiBytes) != expectedAnsi {
			t.Errorf("ANSI snapshot content mismatch: expected %q, got %q", expectedAnsi, string(ansiBytes))
		}
		if string(txtBytes) != expectedTxt {
			t.Errorf("TXT snapshot content mismatch: expected %q, got %q", expectedTxt, string(txtBytes))
		}
	})

	// 3. Test matching snapshot (update=false)
	t.Run("matching snapshot", func(t *testing.T) {
		os.Setenv("UPDATE_SNAPSHOTS", "false")

		mt := &mockT{}
		// Test with CRLF, which should be normalized and match the recorded LF snapshot
		content := "\x1b[31mHello\x1b[0m\r\nWorld"
		AssertTUISnapshot(mt, snapshotName, content)

		if len(mt.errors) > 0 || len(mt.fatals) > 0 {
			t.Errorf("expected matching snapshot to pass, but got errors=%v, fatals=%v", mt.errors, mt.fatals)
		}
	})

	// 4. Test ANSI mismatch (same text, different style)
	t.Run("ansi style mismatch", func(t *testing.T) {
		os.Setenv("UPDATE_SNAPSHOTS", "false")

		mt := &mockT{}
		content := "\x1b[32mHello\x1b[0m\nWorld" // green instead of red
		AssertTUISnapshot(mt, snapshotName, content)

		if len(mt.errors) == 0 {
			t.Fatal("expected ANSI mismatch error, but got none")
		}
		if !strings.Contains(mt.errors[0], "ANSI snapshot mismatch") {
			t.Errorf("expected ANSI mismatch error message, got: %s", mt.errors[0])
		}
		if len(mt.fatals) > 0 {
			t.Errorf("unexpected fatal error: %v", mt.fatals)
		}
	})

	// 5. Test TXT mismatch (different text)
	t.Run("txt content mismatch", func(t *testing.T) {
		os.Setenv("UPDATE_SNAPSHOTS", "false")

		mt := &mockT{}
		content := "\x1b[31mHello\x1b[0m\nWorld!" // extra exclamation mark
		AssertTUISnapshot(mt, snapshotName, content)

		if len(mt.errors) == 0 {
			t.Fatal("expected TXT mismatch error, but got none")
		}
		if !strings.Contains(mt.errors[0], "TXT snapshot mismatch") {
			t.Errorf("expected TXT mismatch error message, got: %s", mt.errors[0])
		}
	})
}
