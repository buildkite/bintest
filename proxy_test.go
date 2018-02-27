package bintest_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/buildkite/bintest"
	"github.com/fortytw2/leaktest"
)

func proxyTearDown(t *testing.T) func() {
	leakTest := leaktest.Check(t)
	return func() {
		leakTest()
	}
}

func TestProxyWithStdin(t *testing.T) {
	defer proxyTearDown(t)()

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	outBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path)
	cmd.Stdin = strings.NewReader("This is my stdin\n")
	cmd.Stdout = outBuf
	cmd.Stderr = os.Stderr

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	fmt.Fprintln(call.Stdout, "Copied to stdout")

	_, err = io.Copy(call.Stdout, call.Stdin)
	if err != nil {
		t.Fatal(err)
	}
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
	defer proxyTearDown(t)()

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	outBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, "test", "arguments")
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
	defer proxyTearDown(t)()

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	errBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stderr = errBuf

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

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
	defer proxyTearDown(t)()

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	if !reflect.DeepEqual(call.Args[1:], []string{"test", "arguments"}) {
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
	defer proxyTearDown(t)()

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	call.Exit(0)

	// wait for the command to finish
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestProxyWithLotsOfOutput(t *testing.T) {
	defer proxyTearDown(t)()

	var expected string
	for i := 0; i < 10; i++ {
		expected += strings.Repeat("llamas", 10)
	}

	actual := &bytes.Buffer{}

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stdout = actual

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

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
	defer proxyTearDown(t)()

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

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
	defer proxyTearDown(t)()

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = os.Stat(proxy.Path); os.IsNotExist(err) {
		t.Fatalf("%s doesn't exist, but should: %v", proxy.Path, err)
	}

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

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
	defer proxyTearDown(t)()

	tempDir, err := ioutil.TempDir("", "proxy-wd-test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	cmd := exec.Command(proxy.Path, "test", "arguments")
	cmd.Dir = tempDir

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

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
	defer proxyTearDown(t)()

	echoCmd := `/bin/echo`
	if runtime.GOOS == `windows` {
		// Question every life choice that has lead you to want to understand the below
		// https://ss64.com/nt/syntax-esc.html
		echoCmd = writeBatchFile(t, "echo.bat", []string{
			`@ECHO OFF`,
			`Set _string=%~1`,
			`Echo %_string%`,
		})
	}

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	outBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path, `hello world`)
	cmd.Stdout = outBuf
	cmd.Stderr = os.Stderr

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	call.Passthrough(echoCmd)

	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	out := normalizeNewlines(outBuf.String())
	if expected := "hello world\n"; out != expected {
		t.Fatalf("Expected stdout to be %q, got %q", expected, out)
	}
}

func TestProxyWithPassthroughWithStdin(t *testing.T) {
	defer proxyTearDown(t)()

	catCmd := `/bin/cat`
	if runtime.GOOS == `windows` {
		catCmd = writeBatchFile(t, "cat.bat", []string{
			`@ECHO OFF`,
			`FIND/V ""`,
		})
	}

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	inBuf := bytes.NewBufferString(normalizeNewlines("hello world\n"))
	outBuf := &bytes.Buffer{}

	cmd := exec.Command(proxy.Path)
	cmd.Stdin = inBuf
	cmd.Stdout = outBuf

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	call.Passthrough(catCmd)

	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	out := normalizeNewlines(outBuf.String())

	if expected := "hello world\n"; out != expected {
		t.Fatalf("Expected stdout to be %q, got %q", expected, out)
	}
}

func TestProxyWithPassthroughWithFailingCommand(t *testing.T) {
	defer proxyTearDown(t)()

	falseCmd := `/usr/bin/false`
	if runtime.GOOS == `windows` {
		falseCmd = writeBatchFile(t, "false.bat", []string{
			`@ECHO OFF`,
			`EXIT /B 1`,
		})
	}

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	cmd := exec.Command(proxy.Path)

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	call.Passthrough(falseCmd)

	if err = cmd.Wait(); err == nil {
		t.Fatalf("Expected an error")
	}
}

func TestProxyWithPassthroughWithTimeout(t *testing.T) {
	defer proxyTearDown(t)()

	if runtime.GOOS == `windows` {
		t.Skipf("Not implemented for windows")
	}

	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			t.Error(err)
		}
	}()

	b := &bytes.Buffer{}
	cmd := exec.Command(proxy.Path, "100")
	cmd.Stdout = b
	cmd.Stderr = b

	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}

	call := <-proxy.Ch
	call.PassthroughWithTimeout(`/bin/sleep`, time.Millisecond*100)

	if err = cmd.Wait(); err == nil {
		t.Fatalf("Expected an error!")
	}
}

func TestProxyCallingInParallel(t *testing.T) {
	defer proxyTearDown(t)()

	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			proxy, err := bintest.LinkTestBinaryAsProxy(fmt.Sprintf("test%d", i))
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := proxy.Close(); err != nil {
					t.Error(err)
				}
			}()

			cmd := exec.Command(proxy.Path)
			cmd.Env = proxy.Environ()

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

	wg.Wait()
}

func writeBatchFile(t *testing.T, name string, lines []string) string {
	tmpDir, err := ioutil.TempDir("", "batch-files-of-horror")
	if err != nil {
		t.Fatal(err)
	}

	batchfile := filepath.Join(tmpDir, name)
	err = ioutil.WriteFile(batchfile, []byte(strings.Join(lines, "\r\n")), 0600)
	if err != nil {
		t.Fatal(err)
	}

	return batchfile
}

func normalizeNewlines(s string) string {
	return strings.Replace(s, "\r\n", "\n", -1)
}

func BenchmarkCreatingProxies(b *testing.B) {
	for n := 0; n < b.N; n++ {
		proxy, err := bintest.CompileProxy("test")
		if err != nil {
			b.Fatal(err)
		}
		defer func() {
			if err := proxy.Close(); err != nil {
				b.Error(err)
			}
		}()
	}
}

func BenchmarkCallingProxies(b *testing.B) {
	proxy, err := bintest.CompileProxy("test")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := proxy.Close(); err != nil {
			b.Error(err)
		}
	}()

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
		_, _ = io.Copy(call.Stdout, strings.NewReader(expected))
		call.Exit(0)

		b.StopTimer()
		if err = cmd.Wait(); err != nil {
			b.Fatal(err)
		}
	}
}
