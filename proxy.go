package binproxy

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
)

// Proxy connects to a compiled binary to orchestrate it's input/output
type Proxy struct {
	CallFunc

	Path  string
	Calls []*Call

	server *server
}

// New returns a new instance of a Proxy, compiled to path and executing cf on call
func New(path string, cf CallFunc) (*Proxy, error) {
	if !filepath.IsAbs(path) {
		dir, err := ioutil.TempDir("", "binproxy")
		if err != nil {
			return nil, fmt.Errorf("Error creating temp dir: %v", err)
		}
		path = filepath.Join(dir, path)
	}

	p := &Proxy{
		Path:     path,
		CallFunc: cf,
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

func (p *Proxy) call(args []string, env []string) *Call {
	call := startCall(int64(len(p.Calls)+1), args, env, p.CallFunc)
	p.Calls = append(p.Calls, call)
	return call
}

// CallFunc is the logic to execute when a binary is called
type CallFunc func(call *Call)

func startCall(id int64, args []string, env []string, f CallFunc) *Call {
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	inR, inW := io.Pipe()

	c := &Call{
		ID:           id,
		Args:         args,
		Env:          env,
		Stderr:       errW,
		stderrReader: errR,
		Stdout:       outW,
		stdoutReader: outR,
		Stdin:        inR,
		stdinWriter:  inW,
	}

	go func() {
		f(c)

		// stdout and stderr close here, stdin closes in the server
		c.stdoutReader.Close()
		c.stderrReader.Close()
	}()

	return c
}

// Call is created for every call to the proxied binary
type Call struct {
	ID   int64
	Args []string
	Env  []string

	// Stdout is the output writer to send stdout to in the proxied binary
	Stdout io.WriteCloser `json:"-"`

	// Stderr is the output writer to send stdout to in the proxied binary
	Stderr io.WriteCloser `json:"-"`

	// Stdin is the input reader for stdin from the proxied binary
	Stdin io.ReadCloser `json:"-"`

	proxy        *Proxy
	stdoutReader io.ReadCloser
	stderrReader io.ReadCloser
	stdinWriter  io.WriteCloser
	exitCode     int
}

// Exit sets the exit code for the remote binary to exit with
func (c *Call) Exit(code int) {
	c.exitCode = code
}
