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

	"github.com/lox/binproxy"
)

func ExampleNew() {
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

func TestProxyWithNoOutput(t *testing.T) {
	proxy, err := binproxy.New("test", func(c *binproxy.Call) {
	})
	if err != nil {
		log.Fatal(err)
	}

	output, err := exec.Command(proxy.Path).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	if string(output) != "" {
		t.Fatalf("Got unexpected output: %q", string(output))
	}
}

func TestProxyWithLotsOfOutput(t *testing.T) {
	b := &bytes.Buffer{}
	for i := 0; i < 10; i++ {
		b.WriteString(strings.Repeat("llamas", 10))
	}
	expected := b.String()
	fatalCh := make(chan error, 1)

	proxy, err := binproxy.New("test", func(c *binproxy.Call) {
		n, err := io.Copy(c.Stdout, strings.NewReader(expected))
		if err != nil {
			fatalCh <- err
		} else if n != int64(b.Len()) {
			fatalCh <- fmt.Errorf("Wrote %d bytes, expected %d", n, len(expected))
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	actual, err := exec.Command(proxy.Path).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-fatalCh:
		t.Fatal(err)
	default:
	}

	if len(expected) != len(actual) {
		t.Fatalf("Wanted %d bytes of output, got %d", len(expected), len(actual))
	}
}
