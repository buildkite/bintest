package binproxy_test

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/lox/binproxy"
)

func ExampleNew() {
	// create a proxy for the git command that echos some debug
	proxy, err := binproxy.New("git")
	if err != nil {
		log.Fatal(err)
	}

	// call the proxy like a normal binary in the background
	cmd := exec.Command(proxy.Path)
	cmd.Stdout = os.Stdout
	cmd.Start()

	// handle invocations of the proxy binary
	call := <-proxy.Ch
	fmt.Fprintln(call.Stdout, "Llama party! 🎉")
	call.Exit(0)

	// wait for the command to finish
	cmd.Wait()

	// Output: Llama party! 🎉
}

func TestProxyWithStdin(t *testing.T) {
	proxy, err := binproxy.New("test")
	if err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path)
	cmd.Stdin = strings.NewReader("This is my stdin")
	cmd.Stdout = stdout
	cmd.Start()

	call := <-proxy.Ch
	fmt.Fprintln(call.Stdout, "Copied output:")
	io.Copy(call.Stdout, call.Stdin)
	call.Exit(0)

	// wait for the command to finish
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	if stdout.String() != "Copied output:\nThis is my stdin" {
		t.Fatalf("Got unexpected output: %q", stdout.String())
	}
}

func TestProxyWithStdoutAndStderr(t *testing.T) {
	proxy, err := binproxy.New("test")
	if err != nil {
		t.Fatal(err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	if !reflect.DeepEqual(call.Args, []string{"test", "arguments"}) {
		t.Errorf("Unexpected args %#v", call.Args)
		return
	}
	fmt.Fprintln(call.Stdout, "To stdout")
	fmt.Fprintln(call.Stderr, "To stderr")
	call.Exit(0)

	// wait for the command to finish
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	if stdout.String() != "To stdout\n" {
		t.Fatalf("Got unexpected output: %q", stdout.String())
	}

	if stderr.String() != "To stderr\n" {
		t.Fatalf("Got unexpected output: %q", stderr.String())
	}
}

func TestProxyWithNoOutput(t *testing.T) {
	proxy, err := binproxy.New("test")
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Start()

	call := <-proxy.Ch
	call.Exit(0)

	// wait for the command to finish
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestProxyWithLotsOfOutput(t *testing.T) {
	var expected string
	for i := 0; i < 10; i++ {
		expected += strings.Repeat("llamas", 10)
	}

	actual := &bytes.Buffer{}

	proxy, err := binproxy.New("test")
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stdout = actual
	cmd.Start()

	call := <-proxy.Ch
	n, err := io.Copy(call.Stdout, strings.NewReader(expected))
	if err != nil {
		t.Fatal(err)
	} else if n != int64(len(expected)) {
		t.Fatalf("Wrote %d bytes, expected %d", n, len(expected))
	}
	call.Exit(0)

	// wait for the command to finish
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	if len(expected) != actual.Len() {
		t.Fatalf("Wanted %d bytes of output, got %d", len(expected), actual.Len())
	}
}
