package client

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

type Client struct {
	Debug bool
	URL   string
	ID    string

	Args       []string
	WorkingDir string
	Env        []string

	Stdin  io.ReadCloser
	Stdout io.WriteCloser
	Stderr io.WriteCloser
}

func New(ID string, URL string) *Client {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	return &Client{
		URL:        URL,
		ID:         ID,
		Args:       os.Args[1:],
		Env:        os.Environ(),
		WorkingDir: wd,
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	}
}

// Run the client, panics on error and returns an exit code on success
func (c *Client) Run() int {
	c.debugf("Connecting to %s", c.URL)
	defer func() {
		c.debugf("Finished process")
	}()

	// Data sent to the server about the local invocation
	var req = struct {
		ID    string
		Args  []string
		Env   []string
		Dir   string
		Stdin bool
	}{
		c.ID,
		c.Args,
		c.Env,
		c.WorkingDir,
		c.isStdinReadable(),
	}

	// Reply from the server
	var resp struct {
		ID int64
	}

	// We fire off an initial request to start the flow, and expect an integer
	// back that we will use in subsequent requests
	if err := c.postJSON("/", req, &resp); err != nil {
		c.debugf("Err from server: %v", err)
		panic(err)
	}

	c.debugf("Got ID %d from server", resp.ID)

	var wg sync.WaitGroup
	wg.Add(2)

	if c.isStdinReadable() {
		c.debugf("Stdin is readable")
		go func() {
			r, w := io.Pipe()
			wg.Add(1)

			go func() {
				defer wg.Done()
				c.debugf("Copying from Stdin")
				_, err := io.Copy(w, os.Stdin)
				if err != nil {
					c.debugf("Error copying from stdin: %v", err)
					_ = w.CloseWithError(err)
					return
				}
				_ = w.Close()
				c.debugf("Done copying from Stdin")
			}()

			stdinReq, stdinErr := http.NewRequest("POST", fmt.Sprintf("%s/%d/stdin", c.URL, resp.ID), r)
			if stdinErr != nil {
				panic(stdinErr)
			}

			c.debugf("Posting to /stdin")
			resp, err := http.DefaultClient.Do(stdinReq)
			if err != nil {
				panic(err)
			}

			if resp.StatusCode != http.StatusOK {
				panic(fmt.Sprintf("Server response was not OK: %s (%d)",
					resp.Status, resp.StatusCode))
			}
		}()
	} else if c.Stdin != nil {
		c.debugf("Closing stdin, nothing to read")
		_ = c.Stdin.Close()
	} else {
		c.debugf("Skipping nil stdin")
	}

	go func() {
		err := c.getStream(fmt.Sprintf("%d/stdout", resp.ID), c.Stdout, &wg)
		if err != nil {
			panic(err)
		}
	}()

	go func() {
		err := c.getStream(fmt.Sprintf("%d/stderr", resp.ID), c.Stderr, &wg)
		if err != nil {
			panic(err)
		}
	}()

	c.debugf("Waiting for streams to finish")
	wg.Wait()
	c.debugf("Streams finished, waiting for exit code")

	exitCodeResp, err := http.Get(fmt.Sprintf("%s/%d/exitcode", c.URL, resp.ID))
	if err != nil {
		panic(err)
	}

	var exitCode int
	if err = json.NewDecoder(exitCodeResp.Body).Decode(&exitCode); err != nil {
		panic(err)
	}

	c.debugf("Got an exit code of %d", exitCode)
	return exitCode
}

func (c *Client) isStdinReadable() bool {
	if c.Stdin == nil {
		return false
	}

	// check that we have a named pipe with stuff to read
	// See https://stackoverflow.com/a/26567513
	if stdinFile, ok := c.Stdin.(*os.File); ok {
		stat, _ := stdinFile.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			c.debugf("Stdin is a terminal")
			return false
		}

		if stat.Size() > 0 {
			c.debugf("Stdin has %d bytes to read", stat.Size())
			return true
		}
	} else {
		c.debugf("Stdin is a plain io.Reader")
		return true
	}

	return false
}

func (c *Client) debugf(pattern string, args ...interface{}) {
	if c.Debug {
		format := fmt.Sprintf("[client %s] %s", filepath.Base(os.Args[0]), pattern)
		b := bytes.NewBufferString(fmt.Sprintf(format, args...))
		u := fmt.Sprintf("%s/debug", c.URL)

		resp, err := http.Post(u, "text/plain; charset=utf-8", b)
		if err != nil {
			log.Printf("Error posting to debug: %#v", err)
		}
		defer func() {
			_ = resp.Body.Close()
		}()
	}
}

func (c *Client) get(path string) (*http.Response, error) {
	c.debugf("GET /%s", path)

	resp, err := http.Get(c.URL + "/" + path)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Server response was not OK: %s (%d)",
			resp.Status, resp.StatusCode)
	}

	return resp, err
}

func (c *Client) getStream(path string, w io.WriteCloser, wg *sync.WaitGroup) error {
	resp, err := c.get(path)
	if err != nil {
		return err
	}

	go func() {
		c.debugf("Copying from %s", path)
		b, _ := io.Copy(w, resp.Body)
		_ = resp.Body.Close()
		wg.Done()
		c.debugf("Copied %d bytes from %s", b, path)
	}()

	return nil
}

func (c *Client) postJSON(path string, from interface{}, into interface{}) (err error) {
	body := new(bytes.Buffer)
	if err = json.NewEncoder(body).Encode(from); err != nil {
		return err
	}

	c.debugf("POST %s <- json %+v", path, from)

	resp, respErr := http.Post(c.URL, "application/json; charset=utf-8", body)
	if respErr != nil {
		return err
	}
	defer func() {
		if respErr := resp.Body.Close(); respErr != nil {
			err = respErr
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Server response was not OK: %s (%d)",
			resp.Status, resp.StatusCode)
	}

	// Receive the body as JSON
	if err = json.NewDecoder(resp.Body).Decode(&into); err != nil {
		return err
	}

	return nil
}
