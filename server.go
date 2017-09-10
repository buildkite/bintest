package binproxy

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
)

type server struct {
	sync.Mutex
	Listener net.Listener
	Proxy    *Proxy

	handlers map[int64]callHandler
}

type serverRequest struct {
	Args []string
	Env  []string
}

func (s *server) serveRoot(w http.ResponseWriter, r *http.Request) {
	var req serverRequest

	// parse the posted args end env
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.Lock()
	call := s.Proxy.call(req.Args, req.Env)
	s.handlers[call.ID] = callHandler{Call: call}
	s.Unlock()

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(&call)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// log.Printf("[http] %s", r.URL.Path)

	if r.URL.Path == "/" {
		s.serveRoot(w, r)
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
}

func startServer(p *Proxy) (*server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	s := &server{
		Listener: l,
		Proxy:    p,
		handlers: map[int64]callHandler{},
	}

	go http.Serve(l, s)
	return s, nil
}

type callHandler struct {
	*Call
}

func (ch *callHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch path.Base(r.URL.Path) {
	case "stdout":
		_, _ = io.Copy(w, ch.Call.stdoutReader)

	case "stderr":
		_, _ = io.Copy(w, ch.Call.stderrReader)

	case "stdin":
		_, _ = io.Copy(ch.Call.stdinWriter, r.Body)
		r.Body.Close()
		ch.Call.stdinWriter.Close()

	case "exitcode":
		w.Header().Add("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(&ch.Call.exitCode)

	default:
		http.Error(w, "Unhandled request", http.StatusNotFound)
		return
	}
}
