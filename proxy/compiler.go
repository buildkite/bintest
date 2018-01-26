package proxy

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const clientSrc = `package main

import (
	"github.com/lox/bintest/proxy/client"
	"os"
)

var (
	debug  string
	server string
	id     string
)

func main() {
	c := client.New(id, server)

	if debug == "true" {
		c.Debug = true
	}

	os.Exit(c.Run())
}
`

func compile(dest string, src string, vars []string) error {
	args := []string{
		"build", "-i", "-o", dest,
	}

	if len(vars) > 0 {
		args = append(args, "-ldflags")

		for idx, val := range vars {
			vars[idx] = "-X " + val
		}

		if Debug {
			vars = append(vars, "-X main.debug=true")
		}

		args = append(args, strings.Join(vars, " "))
	}

	t := time.Now()

	debugf("[compiler] go %s %s", strings.Join(args, " "), src)
	output, err := exec.Command("go", append(args, src)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Compile of %s failed: %s", src, output)
	}

	debugf("[compiler] Compiled %s in %v", dest, time.Now().Sub(t))
	return nil
}

func compileClient(dest string, vars []string) error {
	dir, err := ioutil.TempDir("", "binproxy")
	if err != nil {
		return fmt.Errorf("Error creating temp dir: %v", err)
	}

	defer os.RemoveAll(dir)

	err = ioutil.WriteFile(filepath.Join(dir, "client.go"), []byte(clientSrc), 0500)
	if err != nil {
		return err
	}

	return compile(dest, filepath.Join(dir, "client.go"), vars)
}
