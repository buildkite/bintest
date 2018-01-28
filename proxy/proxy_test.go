package proxy_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
	"github.com/lox/bintest/proxy"
)

func ExampleCompile() {
	// create a proxy for the git command that echos some debug
	p, err := proxy.Compile("git")
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	// call the proxy like a normal binary in the background
	cmd := exec.Command(p.Path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// windows needs all the environment variables
	cmd.Env = append(os.Environ(), `MY_MESSAGE=Llama party! 🎉`)

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// handle invocations of the proxy binary
	call := <-p.Ch
	fmt.Fprintln(call.Stdout, call.GetEnv(`MY_MESSAGE`))
	call.Exit(0)

	// wait for the command to finish
	cmd.Wait()

	// Output: Llama party! 🎉
}

func tearDown(t *testing.T) func() {
	leakTest := leaktest.Check(t)
	return func() {
		if err := proxy.StopServer(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond * 10)
		leakTest()
	}
}

func TestProxyWithStdin(t *testing.T) {
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	outBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path)
	cmd.Env = []string{}
	cmd.Stdin = strings.NewReader("This is my stdin\n")
	cmd.Stdout = outBuf
	cmd.Stderr = os.Stderr

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	fmt.Fprintln(call.Stdout, "Copied to stdout")
	io.Copy(call.Stdout, call.Stdin)
	call.Exit(0)

	// wait for the command to finish
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	if expected := "Copied to stdout\nThis is my stdin\n"; outBuf.String() != expected {
		t.Fatalf("Expected stdout to be %q, got %q", expected, outBuf.String())
	}
}

func TestProxyWithStdout(t *testing.T) {
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	outBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Env = []string{}
	cmd.Stdout = outBuf
	cmd.Stderr = os.Stderr

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	fmt.Fprintln(call.Stdout, "Yup!")
	call.Exit(0)

	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	if expected := "Yup!\n"; outBuf.String() != expected {
		t.Fatalf("Expected stdout to be %q, got %q", expected, outBuf.String())
	}
}

func TestProxyWithStderr(t *testing.T) {
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	errBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stderr = errBuf
	cmd.Start()

	call := <-proxy.Ch
	fmt.Fprintln(call.Stderr, "Yup!")
	call.Exit(0)

	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	if expected := "Yup!\n"; errBuf.String() != expected {
		t.Fatalf("Expected stderr to be %q, got %q", expected, errBuf.String())
	}
}

func TestProxyWithStdoutAndStderr(t *testing.T) {
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Env = []string{}
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
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
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
	defer tearDown(t)()

	var expected string
	for i := 0; i < 10; i++ {
		expected += strings.Repeat("llamas", 10)
	}

	actual := &bytes.Buffer{}

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

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

func TestProxyWithNonZeroExitCode(t *testing.T) {
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()

	call := <-proxy.Ch
	call.Exit(24)

	// wait for the command to finish
	err = cmd.Wait()

	if exiterr, ok := err.(*exec.ExitError); !ok {
		t.Fatal("Should have gotten an error from wait")
	} else {
		if status, ok := exiterr.Sys().(syscall.WaitStatus); !ok {
			t.Fatalf("Should have gotten an syscall.WaitStatus, got %v", exiterr)
		} else if status.ExitStatus() != 24 {
			t.Fatalf("Expected exit code %d, got %d", 24, status.ExitStatus())
		}
	}
}

func TestProxyCloseRemovesFile(t *testing.T) {
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = os.Stat(proxy.Path); os.IsNotExist(err) {
		t.Fatalf("%s doesn't exist, but should: %v", proxy.Path, err)
	}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Start()

	call := <-proxy.Ch
	call.Exit(0)

	err = cmd.Wait()
	if err != nil {
		t.Fatal(err)
	}

	err = proxy.Close()
	if err != nil {
		t.Fatal(err)
	}

	if _, err = os.Stat(proxy.Path); os.IsExist(err) {
		t.Fatalf("%s still exists, but shouldn't", proxy.Path)
	}
}

func TestProxyGetsWorkingDirectoryFromClient(t *testing.T) {
	defer tearDown(t)()

	tempDir, err := ioutil.TempDir("", "proxy-wd-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Dir = tempDir
	cmd.Start()

	call := <-proxy.Ch
	dir := strings.TrimPrefix(call.Dir, "/private")
	if dir != tempDir {
		t.Fatalf("Expected call dir to be %q, got %q", tempDir, dir)
	}
	call.Exit(0)

	// wait for the command to finish
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestProxyWithPassthroughWithNoStdin(t *testing.T) {
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	outBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, `hello world`)
	cmd.Env = []string{}
	cmd.Stdout = outBuf

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	call.Passthrough(`/bin/echo`)

	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	if expected := "hello world\n"; outBuf.String() != expected {
		t.Fatalf("Expected stdout to be %q, got %q", expected, outBuf.String())
	}
}

func TestProxyWithPassthroughWithStdin(t *testing.T) {
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	inBuf := bytes.NewBufferString("hello world\n")
	outBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path)
	cmd.Env = []string{}
	cmd.Stdin = inBuf
	cmd.Stdout = outBuf

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	call.Passthrough(`/bin/cat`)

	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	if expected := "hello world\n"; outBuf.String() != expected {
		t.Fatalf("Expected stdout to be %q, got %q", expected, outBuf.String())
	}
}

func TestProxyWithPassthroughWithFailingCommand(t *testing.T) {
	defer tearDown(t)()

	proxy, err := proxy.Compile("test")
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Close()

	cmd := exec.Command(proxy.Path)
	cmd.Env = []string{}

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	call.Passthrough(`/usr/bin/false`)

	if err = cmd.Wait(); err == nil {
		t.Fatalf("Expected an error")
	}
}

func TestProxyCallingInParallel(t *testing.T) {
	// defer tearDown(t)()

	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			proxy, err := proxy.Compile(fmt.Sprintf("test%d", i))
			if err != nil {
				t.Fatal(err)
			}
			defer proxy.Close()

			cmd := exec.Command(proxy.Path)
			cmd.Env = []string{}

			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}

			call := <-proxy.Ch
			call.Exit(0)

			if err = cmd.Wait(); err != nil {
				t.Fatal(err)
			}
		}(i)
	}
}

func BenchmarkCreatingProxies(b *testing.B) {
	for n := 0; n < b.N; n++ {
		proxy, err := proxy.Compile("test")
		if err != nil {
			b.Fatal(err)
		}
		defer proxy.Close()
	}
}

func BenchmarkCallingProxies(b *testing.B) {
	proxy, err := proxy.Compile("test")
	if err != nil {
		b.Fatal(err)
	}
	defer proxy.Close()

	var expected string
	for i := 0; i < 1000; i++ {
		expected += strings.Repeat("llamas", 10)
	}

	b.SetBytes(int64(len(expected)))
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		b.StopTimer()
		cmd := exec.Command(proxy.Path, "test", "arguments")
		if err = cmd.Start(); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		call := <-proxy.Ch
		io.Copy(call.Stdout, strings.NewReader(expected))
		call.Exit(0)

		b.StopTimer()
		if err = cmd.Wait(); err != nil {
			b.Fatal(err)
		}
	}
}
