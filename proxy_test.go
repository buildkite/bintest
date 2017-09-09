package binproxy_test

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	binproxy "github.com/lox/go-binproxy"
)

func ExampleFromReadme() {
	// create a proxy for the git command that echos some debug
	proxy, err := binproxy.New("git", func(c *binproxy.Call) {
		fmt.Fprintln(c.Stdout, "Llamas party!")
		c.Exit(0)
	})
	if err != nil {
		log.Fatal(err)
	}

	// call the proxy like a normal binary
	output, err := exec.Command(proxy.Path, "test", "arguments").CombinedOutput()
	if err != nil {
		log.Fatalf("Command failed: %v\n%s", err, output)
	}

	fmt.Printf("%s\n", output)
	// Output: Llamas party!
}

func TestProxyWithStdin(t *testing.T) {
	proxy, err := binproxy.New("test", func(c *binproxy.Call) {
		fmt.Fprintln(c.Stdout, "Copied output:")
		io.Copy(c.Stdout, c.Stdin)
	})
	if err != nil {
		log.Fatal(err)
	}

	output := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stdin = strings.NewReader("This is my stdin")
	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if output.String() != "Copied output:\nThis is my stdin" {
		t.Fatalf("Got unexpected output: %q", output.String())
	}
}

func TestProxyWithStdoutAndStderr(t *testing.T) {
	proxy, err := binproxy.New("test", func(c *binproxy.Call) {
		if !reflect.DeepEqual(c.Args, []string{"test", "arguments"}) {
			t.Fatalf("Unexpected args %#v", c.Args)
		}
		fmt.Fprintln(c.Stdout, "To stdout")
		fmt.Fprintln(c.Stdout, "To stderr")
	})
	if err != nil {
		log.Fatal(err)
	}

	output := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if output.String() != "To stdout\nTo stderr\n" {
		t.Fatalf("Got unexpected output: %q", output.String())
	}
}
