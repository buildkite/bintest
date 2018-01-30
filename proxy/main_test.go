package proxy_test

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/lox/bintest/proxy"
	"github.com/lox/bintest/proxy/client"
)

func ExampleLinkTestBinaryAsProxy() {
	// create a proxy for the git command that echos some debug
	p, err := proxy.LinkTestBinaryAsProxy("git")
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	// call the proxy like a normal binary in the background
	cmd := exec.Command(p.Path, "rev-parse")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// windows needs all the environment variables
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, p.Environ()...)
	cmd.Env = append(cmd.Env, `MY_MESSAGE=Llama party! 🎉`)

	log.Printf("%#v", cmd.Env)

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

func TestMain(m *testing.M) {
	if filepath.Base(os.Args[0]) != `proxy.test` {
		os.Exit(client.NewFromEnv().Run())
	}

	code := m.Run()
	os.Exit(code)
}
