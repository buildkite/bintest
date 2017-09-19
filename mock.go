package bintest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/lox/bintest/proxy"
)

const (
	InfiniteTimes = -1
)

// TestingT is an interface for *testing.T
type TestingT interface {
	Logf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// Mock provides a wrapper around a Proxy for testing
type Mock struct {
	sync.Mutex

	// Name of the binary
	Name string

	// Path to the bintest binary
	Path string

	// Actual invocations that occurred
	invocations []Invocation

	// The executions expected of the binary
	expected []*Expectation

	// A list of middleware functions to call before invocation
	before []func(i Invocation) error

	// Whether to ignore unexpected calls
	ignoreUnexpected bool

	// The related proxy
	proxy *proxy.Proxy

	// A command to passthrough execution to
	passthroughPath string
}

// Mock returns a new Mock instance, or fails if the bintest fails to compile
func NewMock(path string) (*Mock, error) {
	m := &Mock{}

	proxy, err := proxy.New(path)
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

func (m *Mock) invoke(call *proxy.Call) {
	m.Lock()
	defer m.Unlock()

	debugf("Handling invocation for %s %s", m.Name, call.Args)

	var invocation = Invocation{
		Args: call.Args,
		Env:  call.Env,
	}

	for _, beforeFunc := range m.before {
		if err := beforeFunc(invocation); err != nil {
			fmt.Fprintf(call.Stderr, "\033[31m🚨 Error: %v\033[0m\n", err)
			call.Exit(1)
			return
		}
	}

	expected, err := m.findMatchingExpectation(call.Args...)
	if err != nil {
		m.invocations = append(m.invocations, invocation)
		if m.ignoreUnexpected {
			call.Exit(0)
		} else {
			fmt.Fprintf(call.Stderr, "\033[31m🚨 Error: %v\033[0m\n", err)
			call.Exit(1)
		}
		return
	}

	expected.Lock()
	defer expected.Unlock()

	invocation.Expectation = expected

	if m.passthroughPath != "" {
		call.Exit(m.invokePassthrough(m.passthroughPath, call))
	} else if expected.passthroughPath != "" {
		call.Exit(m.invokePassthrough(expected.passthroughPath, call))
	} else if expected.callFunc != nil {
		expected.callFunc(call)
	} else {
		_, _ = io.Copy(call.Stdout, expected.writeStdout)
		_, _ = io.Copy(call.Stderr, expected.writeStderr)
		call.Exit(expected.exitCode)
	}

	expected.totalCalls++
	m.invocations = append(m.invocations, invocation)
}

func (m *Mock) invokePassthrough(path string, call *proxy.Call) int {
	debugf("Passing through to %s %v", path, call.Args)
	cmd := exec.Command(path, call.Args...)
	cmd.Env = call.Env
	cmd.Stdout = call.Stdout
	cmd.Stderr = call.Stderr
	cmd.Stdin = call.Stdin
	cmd.Dir = call.Dir

	var waitStatus syscall.WaitStatus
	if err := cmd.Run(); err != nil {
		debugf("Exited with error: %v", err)
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus = exitError.Sys().(syscall.WaitStatus)
			return waitStatus.ExitStatus()
		} else {
			panic(err)
		}
	}

	debugf("Exited with 0")
	return 0
}

// PassthroughToLocalCommand executes the mock name as a local command (looked up in PATH) and then passes
// the result as the result of the mock. Useful for assertions that commands happen, but where
// you want the command to actually be executed.
func (m *Mock) PassthroughToLocalCommand() *Mock {
	m.Lock()
	defer m.Unlock()
	path, err := exec.LookPath(m.Name)
	if err != nil {
		panic(err)
	}
	m.ignoreUnexpected = true
	m.passthroughPath = path
	return m
}

// IgnoreUnexpectedInvocations allows for invocations without matching call expectations
// to just silently return 0 and no output
func (m *Mock) IgnoreUnexpectedInvocations() *Mock {
	m.Lock()
	defer m.Unlock()
	m.ignoreUnexpected = true
	return m
}

// Before adds a middleware that is run before the Invocation is dispatched
func (m *Mock) Before(f func(i Invocation) error) *Mock {
	m.Lock()
	defer m.Unlock()
	if m.before == nil {
		m.before = []func(i Invocation) error{f}
	} else {
		m.before = append(m.before, f)
	}
	return m
}

// Expect creates an expectation that the mock will be called with the provided args
func (m *Mock) Expect(args ...interface{}) *Expectation {
	m.Lock()
	defer m.Unlock()
	ex := &Expectation{
		parent:           m,
		arguments:        Arguments(args),
		writeStderr:      &bytes.Buffer{},
		writeStdout:      &bytes.Buffer{},
		expectedCallsMin: 1,
		expectedCallsMax: 1,
		passthroughPath:  m.passthroughPath,
	}
	m.expected = append(m.expected, ex)
	return ex
}

func (m *Mock) findMatchingExpectation(args ...string) (*Expectation, error) {
	var possibleMatches = []*Expectation{}

	// log.Printf("Trying to match call [%s %s]", m.Name, formatStrings(args))
	for _, expectation := range m.expected {
		expectation.RLock()
		defer expectation.RUnlock()
		// log.Printf("Comparing to [%s]", expectation.String())
		if match, _ := expectation.arguments.Match(args...); match {
			// log.Printf("Matched args")
			possibleMatches = append(possibleMatches, expectation)
		}
	}

	// log.Printf("Found %d possible matches for [%s %s]", len(possibleMatches), m.Name, formatStrings(args))

	for _, expectation := range possibleMatches {
		if expectation.expectedCallsMax == InfiniteTimes || expectation.totalCalls < expectation.expectedCallsMax {
			// log.Printf("Matched %v", expectation)
			return expectation, nil
		}
	}

	if len(possibleMatches) > 0 {
		return nil, fmt.Errorf("Call count didn't match possible expectations for [%s %s]", m.Name, formatStrings(args))
	}

	// log.Printf("No match found")
	return nil, fmt.Errorf("No matching expectation found for [%s %s]", m.Name, formatStrings(args))
}

// ExpectAll is a shortcut for adding lots of expectations
func (m *Mock) ExpectAll(argSlices [][]interface{}) {
	for _, args := range argSlices {
		m.Expect(args...)
	}
}

// Check that all assertions are met and that there aren't invocations that don't match expectations
func (m *Mock) Check(t TestingT) bool {
	m.Lock()
	defer m.Unlock()

	if len(m.expected) == 0 {
		return true
	}

	var failedExpectations, unexpectedInvocations int

	// first check that everything we expect
	for _, expected := range m.expected {
		count := m.countCalls(expected.arguments)
		if expected.expectedCallsMin != InfiniteTimes && count < expected.expectedCallsMin {
			t.Logf("Expected %s %s to be called at least %d times, got %d",
				m.Name, expected.arguments.String(), expected.expectedCallsMin, count,
			)
			failedExpectations++
		} else if expected.expectedCallsMax != InfiniteTimes && count > expected.expectedCallsMax {
			t.Logf("Expected %s %s to be called at most %d times, got %d",
				m.Name, expected.arguments.String(), expected.expectedCallsMax, count,
			)
			failedExpectations++
		}
	}

	if failedExpectations > 0 {
		t.Errorf("Not all expectations were met (%d out of %d)",
			len(m.expected)-failedExpectations,
			len(m.expected))
	}

	// next check if we have invocations without expectations
	if !m.ignoreUnexpected {
		for _, invocation := range m.invocations {
			if invocation.Expectation == nil {
				t.Logf("Unexpected call to %s %s",
					m.Name, formatStrings(invocation.Args))
				unexpectedInvocations++
			}
		}

		if unexpectedInvocations > 0 {
			t.Errorf("More invocations than expected (%d vs %d)",
				unexpectedInvocations,
				len(m.invocations))
		}
	}

	return unexpectedInvocations == 0 && failedExpectations == 0
}

func (m *Mock) countCalls(args Arguments) (count int) {
	for _, invocation := range m.invocations {
		if match, _ := args.Match(invocation.Args...); match {
			count++
		}
	}
	return count
}

func (m *Mock) CheckAndClose(t TestingT) error {
	if err := m.proxy.Close(); err != nil {
		return err
	}
	if !m.Check(t) {
		return errors.New("Assertion checks failed")
	}
	return nil
}

func (m *Mock) Close() error {
	debugf("Closing mock")
	return m.proxy.Close()
}

// Expectation is used for setting expectations
type Expectation struct {
	sync.RWMutex

	parent *Mock

	// Holds the arguments of the method.
	arguments Arguments

	// The exit code to return
	exitCode int

	// The command to execute and return the results of
	passthroughPath string

	// The function to call when executed
	callFunc func(*proxy.Call)

	// Amount of times this call has been called
	totalCalls int

	// Times expected to be called
	expectedCallsMin, expectedCallsMax int

	// Buffers to copy to stdout and stderr
	writeStdout, writeStderr *bytes.Buffer
}

func (e *Expectation) Times(expect int) *Expectation {
	return e.MinTimes(expect).MaxTimes(expect)
}

func (e *Expectation) MinTimes(expect int) *Expectation {
	e.Lock()
	defer e.Unlock()
	if expect == InfiniteTimes {
		expect = 0
	}
	e.expectedCallsMin = expect
	return e
}

func (e *Expectation) MaxTimes(expect int) *Expectation {
	e.Lock()
	defer e.Unlock()
	e.expectedCallsMax = expect
	return e
}

func (e *Expectation) Optionally() *Expectation {
	e.Lock()
	defer e.Unlock()
	e.expectedCallsMin = 0
	return e
}

func (e *Expectation) Once() *Expectation {
	return e.Times(1)
}

func (e *Expectation) NotCalled() *Expectation {
	return e.Times(0)
}

func (e *Expectation) AndExitWith(code int) *Expectation {
	e.Lock()
	defer e.Unlock()
	e.exitCode = code
	e.passthroughPath = ""
	return e
}

func (e *Expectation) AndWriteToStdout(s string) *Expectation {
	e.Lock()
	defer e.Unlock()
	e.writeStdout.WriteString(s)
	e.passthroughPath = ""
	return e
}

func (e *Expectation) AndWriteToStderr(s string) *Expectation {
	e.Lock()
	defer e.Unlock()
	e.writeStderr.WriteString(s)
	e.passthroughPath = ""
	return e
}

func (e *Expectation) AndPassthroughToLocalCommand(path string) *Expectation {
	e.Lock()
	defer e.Unlock()
	e.passthroughPath = path
	return e
}

func (e *Expectation) AndCallFunc(f func(*proxy.Call)) *Expectation {
	e.Lock()
	defer e.Unlock()
	e.callFunc = f
	e.passthroughPath = ""
	return e
}

func (e *Expectation) String() string {
	return fmt.Sprintf("%s %s", e.parent.Name, e.arguments.String())
}

// Invocation is a call to the binary
type Invocation struct {
	Args        []string
	Env         []string
	Expectation *Expectation
}

// ExpectEnv asserts that certain environment vars/values exist, otherwise
// an error is reported to T and a matching error is returned (for Before)
func ExpectEnv(t *testing.T, environ []string, expect ...string) error {
	for _, e := range expect {
		pair := strings.Split(e, "=")
		actual, ok := GetEnv(pair[0], environ)
		if !ok {
			err := fmt.Errorf("Expected %s, %s wasn't set in environment", e, pair[0])
			t.Error(err)
			return err
		}
		if actual != pair[1] {
			err := fmt.Errorf("Expected %s, got %q", e, pair[1])
			t.Error(err)
			return err
		}
	}
	return nil
}

// GetEnv returns the value for a given env in the invocation
func GetEnv(key string, environ []string) (string, bool) {
	for _, e := range environ {
		pair := strings.Split(e, "=")
		if strings.ToUpper(pair[0]) == strings.ToUpper(key) {
			return pair[1], true
		}
	}
	return "", false
}

var (
	Debug bool
)

func debugf(pattern string, args ...interface{}) {
	if Debug {
		log.Printf("[mock] "+pattern, args...)
	}
}
