package bintest_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildkite/bintest/v3"
)

func ExampleLinkTestBinaryAsProxy() {
	// create a proxy for the git command that echos some debug
	p, err := bintest.LinkTestBinaryAsProxy("git")
	if err != nil {
		log.Fatal(err)
	}

	// call the proxy like a normal binary in the background
	cmd := exec.Command(p.Path, "rev-parse")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// windows needs all the environment variables
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, p.Environ()...)
	cmd.Env = append(cmd.Env, `MY_MESSAGE=Llama party! 🎉`)

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// handle invocations of the proxy binary
	call := <-p.Ch
	fmt.Fprintln(call.Stdout, call.GetEnv(`MY_MESSAGE`))
	call.Exit(0)

	// wait for the command to finish
	if err := cmd.Wait(); err != nil {
		log.Fatal(err)
	}

	// cleanup the proxy
	if err := p.Close(); err != nil {
		log.Fatal(err)
	}

	// Output: Llama party! 🎉
}

func TestMain(m *testing.M) {
	// flag.BoolVar(&proxy.Debug, "proxy.debug", false, "Whether to show proxy debug")
	// flag.Parse()

	if strings.TrimSuffix(filepath.Base(os.Args[0]), `.exe`) != `bintest.test` {
		os.Exit(bintest.NewClientFromEnv().Run())
	}

	_, err := bintest.StartServer()
	if err != nil {
		fmt.Printf("Failed to start proxy server: %v", err)
		os.Exit(1)
	}

	code := m.Run()
	os.Exit(code)
}
