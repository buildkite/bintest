package testutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestingT is a fake partial testing.T, which implements the
// bintest.TestingT interface for capturing log & error messages.
type TestingT struct {
	Logs   []string
	Errors []string
}

// Logf stores a log message
func (t *TestingT) Logf(format string, args ...interface{}) {
	t.Logs = append(t.Logs, fmt.Sprintf(format, args...))
}

// Errorf stores an error message
func (t *TestingT) Errorf(format string, args ...interface{}) {
	t.Errors = append(t.Errors, fmt.Sprintf(format, args...))
}

// Copy dumps the Logs and Errors into another test context
func (t *TestingT) Copy(dst *testing.T) {
	for _, s := range t.Logs {
		dst.Log(s)
	}
	for _, s := range t.Errors {
		dst.Error(s)
	}
}

// WriteBatchFile writes the given lines as a windows batch file of the given
// name in a new temporary directory.
func WriteBatchFile(t *testing.T, name string, lines []string) string {
	tmpDir, err := os.MkdirTemp("", "batch-files-of-horror")
	if err != nil {
		t.Fatal(err)
	}

	batchfile := filepath.Join(tmpDir, name)
	err = os.WriteFile(batchfile, []byte(strings.Join(lines, "\r\n")), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	return batchfile
}

// NormalizeNewlines converts Windows newlines to Unix style
func NormalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// ClosingBuffer adds Close() to bytes.Buffer, such that it implements the
// io.WriteCloser interface in addition to e.g. fmt.Stringer interfaces that
// bytes.Buffer already implements.
type ClosingBuffer struct {
	bytes.Buffer
}

// Close does nothing
func (bc *ClosingBuffer) Close() error {
	return nil
}
