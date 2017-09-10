package mock

import (
	"fmt"
	"log"
	"path/filepath"
	"testing"

	binproxy "github.com/lox/go-binproxy"
)

const (
	alwaysRepeat = -1
)

type Mock struct {
	*binproxy.Proxy

	// Name of the binary being called
	Name string

	// Represents the executions expected of the binary
	expectedCalls []*Call

	// The tester to use for failures
	t *testing.T
}

func NewMock(path string, t *testing.T) *Mock {
	m := &Mock{
		Name: filepath.Base(path),
		t:    t,
	}
	proxy, err := binproxy.New(path, binproxy.CallFunc(func(c *binproxy.Call) {
		call := m.findExpectedCall(c.Args...)
		if call == nil {
			t.Fatalf("No matching call found")
		}
		log.Printf("Matched: %#v", call)
		call.totalCalls++
	}))
	if err != nil {
		t.Fatal(err)
	}
	m.Proxy = proxy
	return m
}

func (m *Mock) On(args ...interface{}) *Call {
	ex := &Call{
		parent:        m,
		arguments:     Arguments(args),
		repeatability: alwaysRepeat,
	}
	m.expectedCalls = append(m.expectedCalls, ex)
	return ex
}

func (m *Mock) findExpectedCall(args ...string) *Call {
	for _, call := range m.expectedCalls {
		if call.repeatability == alwaysRepeat || call.repeatability >= 0 {
			if match, _ := call.arguments.Match(args...); match {
				return call
			}
		}
	}
	return nil
}

func (m *Mock) AssertExpectations() bool {
	var somethingMissing bool
	var failedExpectations int

	for _, expectedCall := range m.expectedCalls {
		log.Printf("%#v", expectedCall)
		// if !m.methodWasCalled(expectedCall.Method, expectedCall.Arguments) && expectedCall.totalCalls == 0 {
		// 	somethingMissing = true
		// 	failedExpectations++
		// 	t.Logf("\u274C\t%s(%s)", expectedCall.Method, expectedCall.Arguments.String())
		// } else {
		// 	if expectedCall.Repeatability > 0 {
		// 		somethingMissing = true
		// 		failedExpectations++
		// 	} else {
		// 		t.Logf("\u2705\t%s(%s)", expectedCall.Method, expectedCall.Arguments.String())
		// 	}
		// }

	}

	if somethingMissing {
		m.t.Errorf("Not all expectations were met (%d out of %d)",
			len(m.expectedCalls)-failedExpectations,
			len(m.expectedCalls))
	}

	return !somethingMissing
}

// AssertNumberOfCalls asserts that the mock was called expectedCalls times.
func (m *Mock) AssertNumberOfCalls(expectedCalls int) bool {
	if len(m.Proxy.Calls) != expectedCalls {
		m.t.Errorf("Expected number of calls (%d) does not match the actual number of calls (%d).", expectedCalls, len(m.Proxy.Calls))
		return false
	}
	return true
}

// AssertCalled asserts that the mock was called with the provided arguments
func (m *Mock) AssertCalled(arguments ...interface{}) bool {
	for _, call := range m.Proxy.Calls {
		if match, _ := Arguments(arguments).Match(call.Args...); match {
			return true
		}
	}
	// fmt.Sprintf("Should have been called with arguments %v, but was not.",
	// 	methodName, len(arguments,
	// 	))) {
	// t.Logf("%v", m.expectedCalls())
	return false
}

// func (m *Mock) AssertNotCalled(t TestingT, methodName string, arguments ...interface{}) bool {
// 	if !assert.False(t, m.methodWasCalled(methodName, arguments), fmt.Sprintf("The \"%s\" method was called with %d argument(s), but should NOT have been.", methodName, len(arguments))) {
// 		t.Logf("%v", m.expectedCalls())
// 		return false
// 	}
// 	return true
// }

// Call represents a binary execution and is used for setting expectations,
// as well as recording activity.
type Call struct {
	parent *Mock

	// Holds the arguments of the method.
	arguments Arguments

	// The number of times to return the return arguments when setting
	// expectations. -1 means to always return the value.
	repeatability int

	// The exit code to return
	exitCode int

	// The command to execute and return the results of
	proxyTo string

	// Amount of times this call has been called
	totalCalls int
}

func (e *Call) ExitWith(code int) *Call {
	e.exitCode = code
	return e
}

func (e *Call) ProxyTo(binPath string) *Call {
	e.proxyTo = binPath
	return e
}

func (e *Call) WriteToStdout(s string) *Call {
	return e
}

func (e *Call) WriteToStderr(s string) *Call {
	return e
}

func ArgumentsFromStrings(s []string) Arguments {
	args := make([]interface{}, len(s))

	for idx, v := range s {
		args[idx] = v
	}

	return args
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

type Matcher interface {
	Match(s string) (bool, string)
}

type MatcherFunc func(s string) (bool, string)

func (mf MatcherFunc) Match(s string) (bool, string) {
	return mf(s)
}

func MatchAny() Matcher {
	return MatcherFunc(func(s string) (bool, string) {
		return true, ""
	})
}
