package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// Proxy provides a way to programatically respond to invocations of a compiled
// binary that is created
type Proxy struct {
	// Ch is the channel of calls
	Ch chan *Call

	// Path is the full path to the compiled binproxy file
	Path string

	// A count of how many calls have been made
	CallCount int64

	// A temporary directory created for the binary
	tempDir string
}

// Compile generates a mock binary at the provided path. If just a filename is provided a temp
// directory is created.
func Compile(path string) (*Proxy, error) {
	var tempDir string

	if !filepath.IsAbs(path) {
		var err error
		tempDir, err = ioutil.TempDir("", "binproxy")
		if err != nil {
			return nil, fmt.Errorf("Error creating temp dir: %v", err)
		}
		path = filepath.Join(tempDir, path)
	}

	if runtime.GOOS == "windows" && !strings.HasSuffix(path, ".exe") {
		path += ".exe"
	}

	server, err := startServer()
	if err != nil {
		return nil, err
	}

	p := &Proxy{
		Path:    path,
		Ch:      make(chan *Call),
		tempDir: tempDir,
	}

	server.registerProxy(p)

	return p, compileClient(path, []string{
		"main.server=" + server.URL,
	})
}

func (p *Proxy) newCall(pid int, args []string, env []string, dir string) *Call {
	atomic.AddInt64(&p.CallCount, 1)

	return &Call{
		PID:        pid,
		Name:       filepath.Base(p.Path),
		Args:       args,
		Env:        env,
		Dir:        dir,
		exitCodeCh: make(chan int),
		doneCh:     make(chan struct{}),
	}
}

// Close the proxy and remove the temp directory
func (p *Proxy) Close() (err error) {
	close(p.Ch)

	defer func() {
		if p.tempDir != "" {
			if removeErr := os.RemoveAll(p.tempDir); removeErr != nil {
				err = removeErr
			}
		}
	}()
	defer func() {
		serverLock.Lock()
		defer serverLock.Unlock()
		serverInstance.deregisterProxy(p)
	}()
	return err
}

// Call is created for every call to the proxied binary
type Call struct {
	PID  int
	Name string
	Args []string
	Env  []string
	Dir  string

	// Stdout is the output writer to send stdout to in the proxied binary
	Stdout io.WriteCloser `json:"-"`

	// Stderr is the output writer to send stdout to in the proxied binary
	Stderr io.WriteCloser `json:"-"`

	// Stdin is the input reader for stdin from the proxied binary
	Stdin io.ReadCloser `json:"-"`

	exitCodeCh chan int
	doneCh     chan struct{}
	done       uint32
}

func (c *Call) GetEnv(key string) string {
	for _, e := range c.Env {
		pair := strings.Split(e, "=")
		if strings.ToUpper(key) == strings.ToUpper(pair[0]) {
			return pair[1]
		}
	}
	return ""
}

// Exit finishes the call and the proxied binary returns the exit code
func (c *Call) Exit(code int) {
	if !atomic.CompareAndSwapUint32(&c.done, 0, 1) {
		panic("Can't call Exit() on a Call that is already finished")
	}

	c.debugf("Sending exit code %d to server", code)

	_ = c.Stderr.Close()
	_ = c.Stdout.Close()

	// send the exit code to the server
	c.exitCodeCh <- code

	// wait for the client to get it
	<-c.doneCh
}

// Fatal exits the call and returns the passed error. If it's a exec.ExitError the exit code is used
func (c *Call) Fatal(err error) {
	c.debugf("Fatal error: %v", err)
	fmt.Fprintf(c.Stderr, "Fatal error: %v", err)

	if exitError, ok := err.(*exec.ExitError); ok {
		c.Exit(exitError.Sys().(syscall.WaitStatus).ExitStatus())
	} else {
		c.Exit(1)
	}
}

// Passthrough invokes another local binary and returns the results
func (c *Call) Passthrough(path string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.passthrough(ctx, path)
}

// PassthroughWithTimeout invokes another local binary and returns the results, if execution doesn't finish
// before the timeout the command is killed and an error is returned
func (c *Call) PassthroughWithTimeout(path string, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c.passthrough(ctx, path)
}

func (c *Call) passthrough(ctx context.Context, path string) {
	start := time.Now()
	ticker := time.NewTicker(time.Second)

	defer func() {
		c.debugf("Passthrough to %s %v finished in %v", path, c.Args, time.Now().Sub(start))
		ticker.Stop()
	}()

	c.debugf("Passing call through to %s %v", path, c.Args)
	cmd := exec.CommandContext(ctx, path, c.Args[1:]...)
	cmd.Env = c.Env
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr
	cmd.Stdin = c.Stdin
	cmd.Dir = c.Dir

	if err := cmd.Start(); err != nil {
		c.Fatal(err)
		return
	}

	// Print progress on execution to make debugging easier. We need to check the context because
	// stopping the ticker won't actually close the
	go func() {
		for {
			select {
			case <-ctx.Done():
				c.debugf("Context is done")
				return
			case <-ticker.C:
				c.debugf("Passthrough %s %v has been running for %v", path, c.Args, time.Now().Sub(start))
			}
		}
	}()

	c.debugf("Waiting for command to finish")
	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			c.debugf("Command exceeded deadline")
			c.Fatal(errors.New("Command exceeded deadline and was killed"))
			return
		}
		c.Fatal(err)
		return
	}

	c.Exit(0)
}

// Returns true if the call is done, doesn't block and is thread-safe
func (c *Call) IsDone() bool {
	return atomic.LoadUint32(&c.done) == 1
}

func (c *Call) debugf(pattern string, args ...interface{}) {
	debugf(fmt.Sprintf("[call %d] %s", c.PID, pattern), args...)
}

var (
	Debug bool
)

func debugf(pattern string, args ...interface{}) {
	if Debug {
		log.Printf(pattern, args...)
	}
}
