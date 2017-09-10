package binproxy

import (
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"sync"
)

type Proxy struct {
	sync.Mutex
	CallFunc

	Path  string
	Calls []*Call

	server *server
}

func New(path string, cf CallFunc) (*Proxy, error) {
	if !filepath.IsAbs(path) {
		dir, err := ioutil.TempDir("", "binproxy")
		if err != nil {
			return nil, fmt.Errorf("Error creating temp dir: %v")
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

func (p *Proxy) Call(args []string, env []string) *Call {
	p.Lock()
	defer p.Unlock()

	call := startCall(int64(len(p.Calls)+1), args, env, p.CallFunc)
	call.Proxy = p

	p.Calls = append(p.Calls, call)
	return call
}

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
		c.stdoutReader.Close()
		c.stderrReader.Close()
	}()

	return c
}

type Call struct {
	ID   int64
	Args []string
	Env  []string

	Stdout io.WriteCloser `json:"-"`
	Stderr io.WriteCloser `json:"-"`
	Stdin  io.ReadCloser  `json:"-"`
	Proxy  *Proxy         `json:"-"`

	stdoutReader io.ReadCloser
	stderrReader io.ReadCloser
	stdinWriter  io.WriteCloser
	exitCode     int
}

func (c *Call) Exit(code int) {
	c.exitCode = code
}
