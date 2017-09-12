package binproxy

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

	// The http server the proxy runs
	server *server
}

// New returns a new instance of a Proxy with a compiled binary
func New(path string) (*Proxy, error) {
	var tempDir string

	if !filepath.IsAbs(path) {
		var err error
		tempDir, err = ioutil.TempDir("", "binproxy")
		if err != nil {
			return nil, fmt.Errorf("Error creating temp dir: %v", err)
		}
		path = filepath.Join(tempDir, path)
	}

	p := &Proxy{
		Path:    path,
		Ch:      make(chan *Call),
		tempDir: tempDir,
	}

	var err error
	p.server, err = startServer(p)
	if err != nil {
		return nil, err
	}

	err = compileClient(path, []string{
		"main.server=" + p.server.Listener.Addr().String(),
	})
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Proxy) newCall(args []string, env []string) *Call {
	return &Call{
		ID:         atomic.AddInt64(&p.CallCount, 1),
		Args:       args,
		Env:        env,
		exitCodeCh: make(chan int),
		doneCh:     make(chan struct{}),
	}
}

// Close the proxy and remove the compiled file
func (p *Proxy) Close() error {
	if p.tempDir != "" {
		defer os.RemoveAll(p.tempDir)
	}
	return p.server.Listener.Close()
}

// Call is created for every call to the proxied binary
type Call struct {
	sync.Mutex

	ID   int64
	Args []string
	Env  []string

	// Stdout is the output writer to send stdout to in the proxied binary
	Stdout io.WriteCloser `json:"-"`

	// Stderr is the output writer to send stdout to in the proxied binary
	Stderr io.WriteCloser `json:"-"`

	// Stdin is the input reader for stdin from the proxied binary
	Stdin io.ReadCloser `json:"-"`

	// proxy      *Proxy
	exitCodeCh chan int
	doneCh     chan struct{}
}

// Exit finishes the call and the proxied binary returns the exit code
func (c *Call) Exit(code int) {
	c.Stderr.Close()
	c.Stdout.Close()

	// send the exit code to the server
	c.exitCodeCh <- code

	// wait for the client to get it
	<-c.doneCh
}
