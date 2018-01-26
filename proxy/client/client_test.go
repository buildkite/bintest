package client_test

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lox/bintest/proxy/client"
)

func TestClient(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case `/`:
			fmt.Fprintln(w, `{"ID": 1234567}`)
		case `/1234567/stdout`:
			fmt.Fprintln(w, `Success (stdout)!`)
		case `/1234567/stderr`:
			fmt.Fprintln(w, `Success (stderr)!`)
		case `/1234567/exitcode`:
			fmt.Fprintln(w, `0`)
		default:
			http.Error(w,
				fmt.Sprintf("Unhandled request url %s", r.URL.Path),
				http.StatusNotFound)
		}
	}))
	defer ts.Close()

	stdout := &closingBuffer{}
	stderr := &closingBuffer{}

	c := client.Client{
		URL:    ts.URL,
		ID:     "myproxy",
		Args:   []string{"llamas"},
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

type closingBuffer struct {
	bytes.Buffer
}

func (cb *closingBuffer) Close() error {
	return nil
}
