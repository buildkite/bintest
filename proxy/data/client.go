package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
)

var (
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
	wg.Add(3)

	// handle stdin
	go func() {
		defer wg.Done()
		r, w := io.Pipe()

		go func() {
			_, copyErr := io.Copy(w, os.Stdin)
			if copyErr != nil {
				w.CloseWithError(err)
			}
			w.Close()
		}()

		stdinReq, stdinErr := http.NewRequest("POST", fmt.Sprintf("%s%d/stdin", u, resp.ID), r)
		if stdinErr != nil {
			panic(stdinErr)
		}

		_, err = http.DefaultClient.Do(stdinReq)
		if err != nil {
			panic(err)
		}
	}()

	// handle stdout
	go func() {
		stdout, stdoutErr := http.Get(fmt.Sprintf("%s%d/stdout", u, resp.ID))
		if stdoutErr != nil {
			panic(stdoutErr)
		}

		go func() {
			io.Copy(os.Stdout, stdout.Body)
			stdout.Body.Close()
			wg.Done()
		}()
	}()

	// handle stderr
	go func() {
		stderr, stderrErr := http.Get(fmt.Sprintf("%s%d/stderr", u, resp.ID))
		if stderrErr != nil {
			panic(stderrErr)
		}

		go func() {
			io.Copy(os.Stderr, stderr.Body)
			stderr.Body.Close()
			wg.Done()
		}()
	}()

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
