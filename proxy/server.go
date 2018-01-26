package proxy

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	serverInstance *server
	serverLock     sync.Mutex
)

func startServer() (*server, error) {
	serverLock.Lock()
	defer serverLock.Unlock()

	if serverInstance == nil {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}

		s := &server{
			Listener: l,
			URL:      "http://" + l.Addr().String(),

			proxies:  map[string]*Proxy{},
			handlers: map[int64]callHandler{},
		}

		debugf("[server] Starting server on %s", l.Addr().String())
		go func() {
			_ = http.Serve(l, s)
		}()

		serverInstance = s
	}

	return serverInstance, nil
}

func stopServer() error {
	serverLock.Lock()
	defer serverLock.Unlock()

	if serverInstance != nil {
		debugf("[server] Stopping server on %s", serverInstance.Addr().String())
		_ = serverInstance.Close()
		serverInstance = nil
	}

	return nil
}

type server struct {
	sync.Mutex
	net.Listener
	URL string

	proxies  map[string]*Proxy
	handlers map[int64]callHandler
}

func (s *server) registerProxy(p *Proxy) (string, error) {
	s.Lock()
	defer s.Unlock()

	id := fmt.Sprintf("%x", sha1.Sum([]byte(p.Path)))

	debugf("[server] Registering proxy %s as %s", p.Path, id)
	s.proxies[id] = p

	return id, nil
}

func (s *server) deregisterProxy(p *Proxy) error {
	s.Lock()
	defer s.Unlock()

	id := fmt.Sprintf("%x", sha1.Sum([]byte(p.Path)))

	debugf("[server] Deregistering proxy %s", id)
	delete(s.proxies, id)

	return nil
}

func (s *server) serveInitialCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    string
		Args  []string
		Env   []string
		Dir   string
		Stdin bool
	}

	// parse the posted args end env
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	debugf("[server] Initial request for %s (%s)", req.ID, r.RemoteAddr)
	s.Lock()

	// find the proxy instance in the server
	proxy, ok := s.proxies[req.ID]
	if !ok {
		debugf("[server] ERROR: No proxy found for %s", req.ID)
		s.Unlock()
		http.Error(w, "No proxy found for "+req.ID, http.StatusNotFound)
		return
	}

	// these pipes connect the call to the various http request/responses
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	inR, inW := io.Pipe()

	// create a custom handler with the id for subsequent requests to hit
	call := proxy.newCall(req.Args, req.Env, req.Dir)
	call.Stdout = outW
	call.Stderr = errW
	call.Stdin = inR

	// close off stdin if it's not going to be provided
	if !req.Stdin {
		debugf("[server] Ignoring stdin, none provided")
		_ = inW.Close()
	}

	s.handlers[call.ID] = callHandler{
		call:   call,
		stdout: outR,
		stderr: errR,
		stdin:  inW,
	}

	s.Unlock()

	// dispatch to whatever handles the call
	proxy.Ch <- call

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(&struct {
		ID int64
	}{
		ID: call.ID,
	})
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	debugf("[server] %s %s", r.Method, r.URL.Path)

	if r.URL.Path == "/" {
		s.serveInitialCall(w, r)
		return
	}

	id, err := strconv.ParseInt(strings.TrimPrefix(path.Dir(r.URL.Path), "/"), 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	ch, ok := s.handlers[id]
	if !ok {
		http.Error(w, "Unknown handler", http.StatusNotFound)
		return
	}

	ch.ServeHTTP(w, r)
	debugf("[server] END %s (%v)", r.URL.Path, time.Now().Sub(start))
}

type callHandler struct {
	sync.WaitGroup
	call   *Call
	stdout *io.PipeReader
	stderr *io.PipeReader
	stdin  *io.PipeWriter
}

func (ch *callHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch path.Base(r.URL.Path) {
	case "stdout":
		debugf("[server] Starting copy of stdout")
		copyPipeWithFlush(w, ch.stdout)
		debugf("[server] Finished copy of stdout")

	case "stderr":
		debugf("[server] Starting copy of stderr")
		copyPipeWithFlush(w, ch.stderr)
		debugf("[server] Finished copy of stderr")

	case "stdin":
		debugf("[server] Starting copy of stdin")
		_, _ = io.Copy(ch.stdin, r.Body)
		_ = r.Body.Close()
		_ = ch.stdin.Close()
		debugf("[server] Finished copy of stdin")

	case "exitcode":
		debugf("[server] Waiting for exitcode to send")
		exitCode := <-ch.call.exitCodeCh
		w.Header().Add("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(&exitCode)
		w.(http.Flusher).Flush()
		debugf("[server] Sending exit code %d to proxy", exitCode)
		ch.call.doneCh <- struct{}{}

	default:
		http.Error(w, "Unhandled request", http.StatusNotFound)
		return
	}
}

func copyPipeWithFlush(res http.ResponseWriter, pipeReader *io.PipeReader) {
	buffer := make([]byte, 1024)
	for {
		n, err := pipeReader.Read(buffer)
		if err != nil {
			pipeReader.Close()
			break
		}

		data := buffer[0:n]
		res.Write(data)
		if f, ok := res.(http.Flusher); ok {
			f.Flush()
		}
		//reset buffer
		for i := 0; i < n; i++ {
			buffer[i] = 0
		}
	}
}
