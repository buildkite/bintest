package proxy

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A single instance of the server is run for each golang process. The server has sessions which then
// have proxy calls within those sessions.

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
		}

		debugf("[server] Starting server on %s", l.Addr().String())
		go func() {
			_ = http.Serve(l, s)
		}()

		serverInstance = s
	}

	return serverInstance, nil
}

// Stop the shared http server instance
func StopServer() error {
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
	net.Listener
	URL string

	proxies      sync.Map
	callHandlers sync.Map
}

func (s *server) registerProxy(p *Proxy) {
	debugf("[server] Registering proxy %s", p.Name)
	s.proxies.Store(p.Name, p)
}

func (s *server) deregisterProxy(p *Proxy) {
	debugf("[server] Deregistering proxy %s", p.Name)
	s.proxies.Delete(p.Name)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/debug" {
		body, _ := ioutil.ReadAll(r.Body)
		_ = r.Body.Close()
		debugf("%s", body)
		return
	}

	start := time.Now()
	debugf("[server] %s %s", r.Method, r.URL.Path)

	if r.URL.Path == `/calls/new` {
		s.handleNewCall(w, r)
		return
	}

	callId, err := strconv.ParseInt(strings.TrimPrefix(path.Dir(r.URL.Path), "/calls/"), 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// dispatch the request to a handler with the given id
	handler, ok := s.callHandlers.Load(callId)
	if !ok {
		http.Error(w, "Unknown handler", http.StatusNotFound)
		return
	}

	handler.(*callHandler).ServeHTTP(w, r)
	debugf("[server] END %s (%v)", r.URL.Path, time.Now().Sub(start))
}

func (s *server) handleNewCall(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string
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

	// find the proxy instance in the server
	proxy, ok := s.proxies.Load(req.Name)
	if !ok {
		debugf("[server] ERROR: No proxy found for %s", req.Name)
		http.Error(w, "No proxy found for "+req.Name, http.StatusNotFound)
		return
	}

	debugf("[server] New call for %s", req.Name)

	// these pipes connect the call to the various http request/responses
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	inR, inW := io.Pipe()

	// create a custom handler with the id for subsequent requests to hit
	call := proxy.(*Proxy).newCall(req.Args, req.Env, req.Dir)
	call.Stdout = outW
	call.Stderr = errW
	call.Stdin = inR

	debugf("[server] Returning call id %d", call.ID)

	// close off stdin if it's not going to be provided
	if !req.Stdin {
		debugf("[server] Ignoring stdin, none provided")
		_ = inW.Close()
	}

	// save the handler for subsequent requests
	s.callHandlers.Store(call.ID, &callHandler{
		call:   call,
		stdout: outR,
		stderr: errR,
		stdin:  inW,
	})

	// dispatch to whatever handles the call
	proxy.(*Proxy).Ch <- call

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(&struct {
		ID int64
	}{
		ID: call.ID,
	})
}

type callHandler struct {
	sync.WaitGroup
	call           *Call
	stdout, stderr *io.PipeReader
	stdin          *io.PipeWriter
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
