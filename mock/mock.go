package mock

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/lox/binproxy"
)

const (
	infinite = -1
)

// Mock provides a wrapper around a binstub for testing
type Mock struct {
	sync.Mutex

	// Name of the binary
	Name string

	// Path to the binproxy binary
	Path string

	// Actual invocations that occurred
	invocations []Invocation

	// The executions expected of the binary
	expected []*Expectation

	// The related proxy
	proxy *binproxy.Proxy

	// A command to passthrough execution to
	passthroughPath string
}

// New returns a new Mock instance, or fails if the binproxy fails to compile
func New(path string) (*Mock, error) {
	m := &Mock{}

	proxy, err := binproxy.New(path)
	if err != nil {
		return nil, err
	}

	m.Name = filepath.Base(proxy.Path)
	m.Path = proxy.Path
	m.proxy = proxy

	go func() {
		for call := range m.proxy.Ch {
			m.invoke(call)
		}
	}()
	return m, nil
}

func (m *Mock) invoke(call *binproxy.Call) {
	m.Lock()
	defer m.Unlock()

	expected := m.findExpectedCall(call.Args...)
	if expected == nil {
		fmt.Fprintf(call.Stderr, "Failed to find an expectation that matches %v", call.Args)
		call.Exit(1)
		return
	}

	expected.Lock()
	defer expected.Unlock()

	if m.passthroughPath == "" {
		_, _ = io.Copy(call.Stdout, expected.writeStdout)
		_, _ = io.Copy(call.Stderr, expected.writeStderr)
		call.Exit(expected.exitCode)
	} else {
		call.Exit(m.invokePassthrough(call))
	}

	expected.totalCalls++

	m.invocations = append(m.invocations, Invocation{
		Args: call.Args,
		Env:  call.Env,
	})
}

func (m *Mock) invokePassthrough(call *binproxy.Call) int {
	cmd := exec.Command(m.passthroughPath, call.Args...)
	cmd.Env = call.Env
	cmd.Stdout = call.Stdout
	cmd.Stderr = call.Stderr
	cmd.Stdin = call.Stdin
	cmd.Dir = call.Dir

	var waitStatus syscall.WaitStatus
	if err := cmd.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus = exitError.Sys().(syscall.WaitStatus)
			return waitStatus.ExitStatus()
		} else {
			panic(err)
		}
	}

	return 0
}

func (m *Mock) PassthroughToLocalCommand() *Mock {
	path, err := exec.LookPath(m.Name)
	if err != nil {
		panic(err)
	}

	m.passthroughPath = path
	return m
}

// Expect creates an expectation that the mock will be called with the provided args
func (m *Mock) Expect(args ...interface{}) *Expectation {
	m.Lock()
	defer m.Unlock()
	ex := &Expectation{
		parent:      m,
		arguments:   Arguments(args),
		writeStderr: &bytes.Buffer{},
		writeStdout: &bytes.Buffer{},
	}
	m.expected = append(m.expected, ex)
	return ex
}

func (m *Mock) findExpectedCall(args ...string) *Expectation {
	for _, call := range m.expected {
		if call.expectedCalls == infinite || call.expectedCalls >= 0 {
			if match, _ := call.arguments.Match(args...); match {
				return call
			}
		}
	}
	return nil
}

func (m *Mock) AssertExpectations(t *testing.T) bool {
	m.Lock()
	defer m.Unlock()

	if len(m.expected) == 0 {
		return true
	}

	var somethingMissing bool
	var failedExpectations int

	for _, expected := range m.expected {
		if !m.wasCalled(expected.arguments) {
			t.Logf("\u274C\t%s(%s)", m.Name, expected.arguments.String())
			somethingMissing = true
			failedExpectations++
		}
	}

	if somethingMissing {
		t.Errorf("Not all expectations were met (%d out of %d)",
			len(m.expected)-failedExpectations,
			len(m.expected))
	}

	return !somethingMissing
}

func (m *Mock) wasCalled(args Arguments) bool {
	for _, invocation := range m.invocations {
		if match, _ := args.Match(invocation.Args...); match {
			return true
		}
	}
	return false
}

// Expectation is used for setting expectations
type Expectation struct {
	sync.Mutex

	parent *Mock

	// Holds the arguments of the method.
	arguments Arguments

	// The exit code to return
	exitCode int

	// The command to execute and return the results of
	proxyTo string

	// Amount of times this call has been called
	totalCalls int

	// Amount of times this is expected to be called
	expectedCalls int

	// Buffers to copy to stdout and stderr
	writeStdout, writeStderr *bytes.Buffer
}

func (e *Expectation) AndExitWith(code int) *Expectation {
	e.Lock()
	defer e.Unlock()
	e.exitCode = code
	return e
}

func (e *Expectation) AndWriteToStdout(s string) *Expectation {
	e.Lock()
	defer e.Unlock()
	e.writeStdout.WriteString(s)
	return e
}

func (e *Expectation) AndWriteToStderr(s string) *Expectation {
	e.Lock()
	defer e.Unlock()
	e.writeStderr.WriteString(s)
	return e
}

func ArgumentsFromStrings(s []string) Arguments {
	args := make([]interface{}, len(s))

	for idx, v := range s {
		args[idx] = v
	}

	return args
}

// Invocation is a call to the binary
type Invocation struct {
	Args []string
	Env  []string
}

type Arguments []interface{}

func (a Arguments) Match(x ...string) (bool, string) {
	for i, expected := range a {
		var formatFail = func(formatter string, args ...interface{}) string {
			return fmt.Sprintf("Argument #%d doesn't match: %s",
				i, fmt.Sprintf(formatter, args...))
		}

		if len(x) <= i {
			return false, formatFail("Expected %q, but missing an argument", expected)
		}

		var actual = x[i]

		if matcher, ok := expected.(Matcher); ok {
			if match, message := matcher.Match(actual); !match {
				return false, formatFail(message)
			}
		} else if s, ok := expected.(string); ok && s != actual {
			return false, formatFail("Expected %q, got %q", expected, actual)
		}
	}
	if len(x) > len(a) {
		return false, fmt.Sprintf(
			"Argument #%d doesn't match: Unexpected extra argument", len(a))
	}

	return true, ""
}

func (a Arguments) String() string {
	var s = make([]string, len(a))
	for idx := range a {
		// log.Printf("#%d %T %#v", idx, a[idx], a[idx])
		switch t := a[idx].(type) {
		case string:
			s[idx] = fmt.Sprintf("%q", t)
		case fmt.Stringer:
			s[idx] = fmt.Sprintf("%s", t.String())
		default:
			s[idx] = fmt.Sprintf("%v", t)
		}
	}
	return strings.Join(s, " ")
}

type Matcher interface {
	fmt.Stringer
	Match(s string) (bool, string)
}

type MatcherFunc struct {
	f   func(s string) (bool, string)
	str string
}

func (mf MatcherFunc) Match(s string) (bool, string) {
	return mf.f(s)
}

func (mf MatcherFunc) String() string {
	return mf.str
}

func MatchAny() Matcher {
	return MatcherFunc{
		f:   func(s string) (bool, string) { return true, "" },
		str: "*",
	}
}
