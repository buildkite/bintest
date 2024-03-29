package bintest_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/buildkite/bintest/v3"
	"github.com/buildkite/bintest/v3/testutil"
)

func TestClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case `/calls/new`:
			w.WriteHeader(http.StatusOK)
		case `/calls/1234567/stdout`:
			fmt.Fprintln(w, `Success (stdout)!`)
		case `/calls/1234567/stderr`:
			fmt.Fprintln(w, `Success (stderr)!`)
		case `/calls/1234567/exitcode`:
			fmt.Fprintln(w, `0`)
		case `/debug`:
			out, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			t.Logf("%s", out)
		default:
			t.Logf("No handler for %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	stdout := &testutil.ClosingBuffer{}
	stderr := &testutil.ClosingBuffer{}

	c := bintest.Client{
		Debug:  false,
		URL:    ts.URL,
		PID:    1234567,
		Args:   []string{"/tmp/llamasbin", "llamas"},
		Stdout: stdout,
		Stderr: stderr,
	}

	if exitCode := c.Run(); exitCode != 0 {
		t.Fatalf("Expected error code of 0, got %d", exitCode)
	}
	if expected := "Success (stdout)!\n"; stdout.String() != expected {
		t.Fatalf("Expected stdout of %q, got %q", expected, stdout.String())
	}
	if expected := "Success (stdout)!\n"; stdout.String() != expected {
		t.Fatalf("Expected stdout of %q, got %q", expected, stdout.String())
	}
}
