package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

func debugf(pattern string, args ...interface{}) {
	if debug == "true" {
		log.Printf(fmt.Sprintf("[%s #%d] ", filepath.Base(os.Args[0]), os.Getpid())+pattern, args...)
	}
}

var (
	debug           string
	server          string
	certPEM, keyPEM string
)

func main() {
	debugf("Connecting to %s", server)
	defer func() {
		debugf("Finished process")
	}()

	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	certPEMDecoded, err := base64.StdEncoding.DecodeString(certPEM)
	if err != nil {
		panic(err)
	}

	keyPEMDecoded, err := base64.StdEncoding.DecodeString(keyPEM)
	if err != nil {
		panic(err)
	}

	client, err := newClient(server, certPEMDecoded, keyPEMDecoded)
	if err != nil {
		panic(err)
	}

	err = client.Initialize(os.Args[1:], os.Environ(), wd)
	if err != nil {
		panic(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	fi, err := os.Stdin.Stat()
	if err != nil {
		panic(err)
	}

	// handle stdin
	go func() {
		r, w := io.Pipe()

		debugf("Stdin has %d bytes", fi.Size())
		if fi.Size() > 0 {
			wg.Add(1)

			go func() {
				defer wg.Done()
				debugf("Copying from Stdin")
				_, copyErr := io.Copy(w, os.Stdin)
				if copyErr != nil {
					debugf("Error copying from stdin: %v", copyErr)
					w.CloseWithError(err)
					return
				}
				w.Close()
				debugf("Done copying from Stdin")
			}()
		} else {
			w.Close()
		}

		if err = client.Stdin(r); err != nil {
			panic(err)
		}
	}()

	// handle stdout
	go func() {
		debugf("Getting /stdout")
		stdout, stdoutErr := client.Stdout()
		if stdoutErr != nil {
			panic(stdoutErr)
		}

		go func() {
			debugf("Copying to Stdout")
			io.Copy(os.Stdout, stdout)
			stdout.Close()
			wg.Done()
			debugf("Finished copying from Stdout")
		}()
	}()

	// handle stderr
	go func() {
		debugf("Getting /stderr")
		stderr, stderrErr := client.Stderr()
		if stderrErr != nil {
			panic(stderrErr)
		}

		go func() {
			debugf("Copying from Stderr")
			io.Copy(os.Stderr, stderr)
			stderr.Close()
			wg.Done()
			debugf("Finished copying from Stderr")
		}()
	}()

	debugf("Waiting for streams to finish")
	wg.Wait()

	exitCode, err := client.ExitCode()
	if err != nil {
		panic(err)
	}

	os.Exit(exitCode)
}

type client struct {
	*http.Client
	u  string
	id int64
}

func newClient(server string, certPEM, keyPEM []byte) (*client, error) {
	// Load our TLS key pair to use for authentication
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("Unable to load cert: %v", err)
	}

	clientCertPool := x509.NewCertPool()
	clientCertPool.AppendCertsFromPEM(certPEM)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      clientCertPool,
	}

	tlsConfig.BuildNameToCertificate()

	return &client{
		Client: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
		u: fmt.Sprintf("https://%s", server),
	}, nil
}

func (c *client) Initialize(Args []string, Env []string, Dir string) error {
	var request = struct {
		Args []string
		Env  []string
		Dir  string
	}{Args, Env, Dir}

	body := new(bytes.Buffer)
	if err := json.NewEncoder(body).Encode(request); err != nil {
		return err
	}

	httpResponse, err := c.Client.Post(c.u+"/", "application/json; charset=utf-8", body)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()

	var response struct {
		ID int64
	}
	if err = json.NewDecoder(httpResponse.Body).Decode(&response); err != nil {
		return err
	}

	c.id = response.ID
	return nil
}

func (c *client) Stdin(r io.Reader) error {
	debugf("Posting to /%d/stdin", c.id)
	_, err := c.Client.Post(fmt.Sprintf("%s/%d/stdin", c.u, c.id), "application/octet-stream", r)
	if err != nil {
		return err
	}
	return nil
}

func (c *client) Stdout() (io.ReadCloser, error) {
	debugf("Getting /%d/stdout", c.id)
	resp, err := c.Client.Get(fmt.Sprintf("%s/%d/stdout", c.u, c.id))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *client) Stderr() (io.ReadCloser, error) {
	debugf("Getting /%d/stderr", c.id)
	resp, err := c.Client.Get(fmt.Sprintf("%s/%d/stderr", c.u, c.id))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *client) ExitCode() (int, error) {
	debugf("Getting /%d/exitcode", c.id)
	exitCodeResp, err := c.Client.Get(fmt.Sprintf("%s/%d/exitcode", c.u, c.id))
	if err != nil {
		return 0, err
	}

	var exitCode int
	if err = json.NewDecoder(exitCodeResp.Body).Decode(&exitCode); err != nil {
		return 0, err
	}

	return exitCode, nil
}
