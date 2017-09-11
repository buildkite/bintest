package binproxy

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"sync"
)

// Proxy connects to a compiled binary to orchestrate it's input/output
type Proxy struct {
	sync.Mutex

	// Ch is the channel of calls
	Ch chan *Call

	// Path is the full path to the compiled binproxy file
	Path string

	// Calls are a history of calls to the Proxy
	Calls []*Call

	server *server
}

// New returns a new instance of a Proxy with a compiled binary
func New(path string) (*Proxy, error) {
	if !filepath.IsAbs(path) {
		dir, err := ioutil.TempDir("", "binproxy")
		if err != nil {
			return nil, fmt.Errorf("Error creating temp dir: %v", err)
		}
		path = filepath.Join(dir, path)
	}

	p := &Proxy{
		Path: path,
		Ch:   make(chan *Call),
	}

	server, err := startServer(p)
	if err != nil {
		return nil, err
	}

	err = compileClient(path, []string{
		"main.server=" + server.Listener.Addr().String(),
	})
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *Proxy) newCall(args []string, env []string) *Call {
	p.Lock()
	defer p.Unlock()

	call := &Call{
		ID:         int64(len(p.Calls) + 1),
		Args:       args,
		Env:        env,
		exitCodeCh: make(chan int),
		doneCh:     make(chan struct{}),
	}

	p.Calls = append(p.Calls, call)
	return call
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

	proxy      *Proxy
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
