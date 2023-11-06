package bintest

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	deadlock "github.com/sasha-s/go-deadlock"
)

const (
	serverEnv = ``
	clientSrc = `package main

import (
	"github.com/buildkite/bintest/v3"
	"os"
)

var (
	debug  string
	server string
)

func main() {
	c := bintest.NewClient(server)

	if debug == "true" {
		c.Debug = true
	}

	os.Exit(c.Run())
}
`
)

var (
	compileCacheInstance *compileCache
	compileLock          deadlock.Mutex
)

func compile(dest string, src string, vars []string) error {
	args := []string{
		"build",
		"-o", dest,
	}

	if len(vars) > 0 || Debug {
		varsCopy := make([]string, len(vars))
		copy(varsCopy, vars)

		args = append(args, "-ldflags")

		for idx, val := range varsCopy {
			varsCopy[idx] = "-X " + val
		}

		if Debug {
			varsCopy = append(varsCopy, "-X main.debug=true")
		}

		args = append(args, strings.Join(varsCopy, " "))
	}

	t := time.Now()

	output, err := exec.Command("go", append(args, src)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Compile of %s failed: %s", src, output)
	}

	debugf("[compiler] Compiled %s in %v", dest, time.Now().Sub(t))
	return nil
}

func compileClient(dest string, vars []string) error {
	serverLock.Lock()
	defer serverLock.Unlock()

	// first off we create a temp dir for caching
	if compileCacheInstance == nil {
		cci, err := newCompileCache()
		if err != nil {
			return err
		}
		compileCacheInstance = cci
	}

	cacheBinaryPath, err := compileCacheInstance.file(vars)
	if err != nil {
		return err
	}

	// if we can, symlink to an existing file in the compile cache
	if compileCacheInstance.IsCached(vars) {
		return replaceSymlink(cacheBinaryPath, dest)
	}

	// we create a temp subdir relative to current dir so that
	// we can make use of gopath / vendor dirs
	dir := fmt.Sprintf(`_bintest_%x`, sha1.Sum([]byte(clientSrc)))
	f := filepath.Join(dir, `main.go`)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	if err := os.WriteFile(f, []byte(clientSrc), 0o500); err != nil {
		return err
	}

	if err := compile(cacheBinaryPath, f, vars); err != nil {
		return err
	}

	if err := os.RemoveAll(dir); err != nil {
		return err
	}

	// Create a symlink to the binary.
	return replaceSymlink(cacheBinaryPath, dest)
}

// To keep the old behaviour of overwriting what was in the destination path,
// replaceSymlink creates a symlink with a temporary name and then renames it
// over the destination path.
func replaceSymlink(oldname, newname string) error {
	tempname := fmt.Sprintf("%s.%x", newname, rand.Int())
	if err := os.Symlink(oldname, tempname); err != nil {
		return err
	}
	return os.Rename(tempname, newname)
}

type compileCache struct {
	Dir string
}

func newCompileCache() (*compileCache, error) {
	cc := &compileCache{}

	var err error
	cc.Dir, err = os.MkdirTemp("", "binproxy")
	if err != nil {
		return nil, fmt.Errorf("Error creating temp dir: %v", err)
	}

	return cc, nil
}

func (c *compileCache) IsCached(vars []string) bool {
	path, err := c.file(vars)
	if err != nil {
		panic(err)
	}

	_, err = os.Stat(path)
	return err == nil
}

func (c *compileCache) Key(vars []string) (string, error) {
	h := sha1.New()

	// add the vars to the hash
	for _, v := range vars {
		if _, err := io.WriteString(h, v); err != nil {
			return "", err
		}
	}
	// factor in client source as well
	_, _ = io.WriteString(h, clientSrc)

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (c *compileCache) file(vars []string) (string, error) {
	if c.Dir == "" {
		return "", errors.New("No compile cache dir set")
	}

	k, err := c.Key(vars)
	if err != nil {
		return "", err
	}

	return filepath.Join(c.Dir, k), nil
}
