package bintest_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/buildkite/bintest/v3"
)

func ExampleCompileProxy() {
	// create a proxy for the git command that echos some debug
	p, err := bintest.CompileProxy("git")
	if err != nil {
		log.Fatal(err)
	}

	// call the proxy like a normal binary in the background
	cmd := exec.Command(p.Path, "rev-parse")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// windows needs all the environment variables
	cmd.Env = append(os.Environ(), `MY_MESSAGE=Llama party! 🎉`)

	if err := cmd.Start(); err != nil {
		_ = p.Close()
		log.Fatal(err)
	}

	// handle invocations of the proxy binary
	call := <-p.Ch
	fmt.Fprintln(call.Stdout, call.GetEnv(`MY_MESSAGE`))
	call.Exit(0)

	// wait for the command to finish
	_ = cmd.Wait()

	if err := p.Close(); err != nil {
		log.Fatal(err)
	}

	// Output: Llama party! 🎉
}

func TestCompileProxy_GoBug22315(t *testing.T) {
	// On Linux (and possibly other Unices), there exists a race condition that
	// manifests when you write and then execute a binary file in a multi-
	// -threaded program. See https://github.com/golang/go/issues/22315.
	// This is debatably a bug in either Linux or Go, but the Go bug has a
	// lengthy discussion.
	//
	// bintest used to operate by copying the compiled binaries around. Copying
	// necessarily requires opening the destination file for writing. Then,
	// bintest provides the path of the freshly-copied binary to the test that
	// is using bintest, which usually immediately executes that binary.
	// Throw in a bit of t.Parallel and you have a recipe for test flakes.

	t.Parallel()
	for i := 0; i < 1000; i++ {
		name := fmt.Sprintf("%s-%d", t.Name(), i)
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			m, err := bintest.NewMock(name)
			if err != nil {
				t.Fatalf("bintest.NewMock(%q) error = %v", name, err)
			}
			defer m.CheckAndClose(t)

			m.Expect().AndExitWith(0)

			// If file copying is used in process, and the race conditions are
			// unfavourable, Run will fail with "text file busy" (ETXTBSY).
			if err := exec.Command(m.Path).Run(); err != nil {
				t.Errorf("exec.Command(%q).Run() = %v", m.Path, err)
			}
		})
	}
}
