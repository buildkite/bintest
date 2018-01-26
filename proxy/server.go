package proxy

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

type server struct {
	sync.Mutex
	net.Listener

	proxy    *Proxy
	handlers map[int64]callHandler
}

type serverRequest struct {
	Args []string
	Env  []string
	Dir  string
}

type serverResponse struct {
	ID int64
}

func (s *server) serveInitialCall(w http.ResponseWriter, r *http.Request) {
	var req serverRequest

	// parse the posted args end env
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	debugf("[server] Initial request from proxy on %s", r.RemoteAddr)

	s.Lock()

	// these pipes connect the call to the various http request/responses
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	inR, inW := io.Pipe()

	// create a custom handler with the id for subsequent requests to hit
	call := s.proxy.newCall(req.Args, req.Env, req.Dir)
	call.Stdout = outW
	call.Stderr = errW
	call.Stdin = inR

	s.handlers[call.ID] = callHandler{
		call:   call,
		stdout: outR,
		stderr: errR,
		stdin:  inW,
	}

	s.Unlock()

	// dispatch to whatever handles the call
	s.proxy.Ch <- call

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(&serverResponse{
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

func startServer(p *Proxy) (*server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	debugf("[server] Started server on %s", l.Addr().String())
	s := &server{
		Listener: l,
		proxy:    p,
		handlers: map[int64]callHandler{},
	}

	debugf("Starting server on %s", l.Addr().String())
	go http.Serve(l, s)
	return s, nil
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
