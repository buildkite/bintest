package proxy

import (
	"crypto/tls"
	"crypto/x509"
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

type handler struct {
	sync.Mutex
	proxy        *Proxy
	callHandlers map[int64]callHandler
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	debugf("[http] START %s", r.URL.Path)

	if r.URL.Path == "/" {
		h.serveInitialCall(w, r)
		return
	}

	id, err := strconv.ParseInt(strings.TrimPrefix(path.Dir(r.URL.Path), "/"), 10, 64)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	ch, ok := h.callHandlers[id]
	if !ok {
		http.Error(w, "Unknown handler", http.StatusNotFound)
		return
	}

	ch.ServeHTTP(w, r)
	debugf("[http] END %s", r.URL.Path)
}

type server struct {
	Addr            string
	certPEM, keyPEM []byte
	*http.Server
}

type serverRequest struct {
	Args []string
	Env  []string
	Dir  string
}

type serverResponse struct {
	ID int64
}

func (h *handler) serveInitialCall(w http.ResponseWriter, r *http.Request) {
	var req serverRequest

	// parse the posted args end env
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.Lock()

	// these pipes connect the call to the various http request/responses
	outR, outW := io.Pipe()
	errR, errW := io.Pipe()
	inR, inW := io.Pipe()

	// create a custom handler with the id for subsequent requests to hit
	call := h.proxy.newCall(req.Args, req.Env, req.Dir)
	call.Stdout = outW
	call.Stderr = errW
	call.Stdin = inR

	h.callHandlers[call.ID] = callHandler{
		call:   call,
		stdout: outR,
		stderr: errR,
		stdin:  inW,
	}

	h.Unlock()

	// dispatch to whatever handles the call
	h.proxy.Ch <- call

	w.Header().Add("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(&serverResponse{
		ID: call.ID,
	})
}

func startServer(p *Proxy) (*server, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	t := time.Now()
	certPEMBlock, keyPEMBlock, err := generateCert(l.Addr().String())
	if err != nil {
		return nil, err
	}

	debugf("Generated TLS certificate for %s in %v", l.Addr().String(), time.Now().Sub(t))

	h := &handler{
		proxy:        p,
		callHandlers: map[int64]callHandler{},
	}

	tlsCert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse TLS certificate: %v", err)
	}

	clientCertPool := x509.NewCertPool()
	if !clientCertPool.AppendCertsFromPEM(certPEMBlock) {
		return nil, fmt.Errorf("Failed to append client certificate")
	}

	tlsConfig := &tls.Config{
		// The certificate that we generated
		Certificates: []tls.Certificate{tlsCert},
		// Reject any TLS certificate that cannot be validated
		ClientAuth: tls.RequireAndVerifyClientCert,
		// Ensure that we only use our "CA" to validate certificates
		ClientCAs: clientCertPool,
		// PFS because we can
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		// Force it server side
		PreferServerCipherSuites: true,
		// TLS 1.2 because we can
		MinVersion: tls.VersionTLS12,
	}

	tlsConfig.BuildNameToCertificate()

	server := &server{
		Addr: l.Addr().String(),
		Server: &http.Server{
			Handler:   h,
			TLSConfig: tlsConfig,
		},
		certPEM: certPEMBlock,
		keyPEM:  keyPEMBlock,
	}

	go func() {
		if err := server.ServeTLS(l, "", ""); err != nil && err != http.ErrServerClosed {
			debugf("Server finished: %v", err)
		}
	}()

	return server, nil
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
		debugf("[call] Starting copy of stdout")
		copyPipeWithFlush(w, ch.stdout)
		debugf("[call] Finished copy of stdout")

	case "stderr":
		debugf("[call] Starting copy of stderr")
		copyPipeWithFlush(w, ch.stderr)
		debugf("[call] Finished copy of stderr")

	case "stdin":
		debugf("[call] Starting copy of stdin")
		_, _ = io.Copy(ch.stdin, r.Body)
		r.Body.Close()
		ch.stdin.Close()
		debugf("[call] Finished copy of stdin")

	case "exitcode":
		debugf("[call] Waiting for exitcode")
		exitCode := <-ch.call.exitCodeCh
		w.Header().Add("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(&exitCode)
		w.(http.Flusher).Flush()
		debugf("[call] Sending exit code")
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
