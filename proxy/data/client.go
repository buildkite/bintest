package main

import (
	"bytes"
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
	debug  string
	server string
)

type request struct {
	Args []string
	Env  []string
	Dir  string
}

type response struct {
	ID int64
}

func main() {
	u := fmt.Sprintf("http://%s/", server)
	debugf("Connecting to %s", u)
	defer func() {
		debugf("Finished process")
	}()

	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	resp, err := jsonPost(u, request{
		Args: os.Args[1:],
		Env:  os.Environ(),
		Dir:  wd,
	})

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

		stdinReq, stdinErr := http.NewRequest("POST", fmt.Sprintf("%s%d/stdin", u, resp.ID), r)
		if stdinErr != nil {
			panic(stdinErr)
		}

		debugf("Posting to /stdin")
		_, err = http.DefaultClient.Do(stdinReq)
		if err != nil {
			panic(err)
		}
	}()

	// handle stdout
	go func() {
		debugf("Getting /stdout")
		stdout, stdoutErr := http.Get(fmt.Sprintf("%s%d/stdout", u, resp.ID))
		if stdoutErr != nil {
			panic(stdoutErr)
		}

		go func() {
			debugf("Copying to Stdout")
			io.Copy(os.Stdout, stdout.Body)
			stdout.Body.Close()
			wg.Done()
			debugf("Finished copying from Stdout")
		}()
	}()

	// handle stderr
	go func() {
		debugf("Getting /stderr")
		stderr, stderrErr := http.Get(fmt.Sprintf("%s%d/stderr", u, resp.ID))
		if stderrErr != nil {
			panic(stderrErr)
		}

		go func() {
			debugf("Copying from Stderr")
			io.Copy(os.Stderr, stderr.Body)
			stderr.Body.Close()
			wg.Done()
			debugf("Finished copying from Stderr")
		}()
	}()

	debugf("Waiting for streams to finish")
	wg.Wait()

	exitCodeResp, err := http.Get(fmt.Sprintf("%s%d/exitcode", u, resp.ID))
	if err != nil {
		panic(err)
	}

	var exitCode int
	if err = json.NewDecoder(exitCodeResp.Body).Decode(&exitCode); err != nil {
		panic(err)
	}

	os.Exit(exitCode)
}

func jsonPost(u string, req request) (*response, error) {
	body := new(bytes.Buffer)
	if err := json.NewEncoder(body).Encode(req); err != nil {
		return nil, err
	}

	// Post a JSON document
	resp, err := http.Post(u, "application/json; charset=utf-8", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Receive the body as JSON
	var decoded response
	if err = json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}

	return &decoded, nil
}
