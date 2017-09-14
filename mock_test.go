package bintest_test

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/lox/bintest"
)

type testingT struct {
	Logs   []string
	Errors []string
}

func (t *testingT) Logf(format string, args ...interface{}) {
	t.Logs = append(t.Logs, fmt.Sprintf(format, args...))
}

func (t *testingT) Errorf(format string, args ...interface{}) {
	t.Errors = append(t.Errors, fmt.Sprintf(format, args...))
}

func TestCallingMockWithNoExpectationsSet(t *testing.T) {
	m, err := bintest.NewMock("test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = exec.Command(m.Path, "blargh").CombinedOutput()
	if err == nil {
		t.Errorf("Expected a failure without any expectations set")
	}

	mt := &testingT{}

	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed (there were none)")
	}
}

func TestCallingMockWithExpectationsSet(t *testing.T) {
	m, err := bintest.NewMock("test")
	if err != nil {
		t.Fatal(err)
	}

	m.Expect("blargh").
		AndWriteToStdout("llamas rock").
		AndExitWith(0)

	out, err := exec.Command(m.Path, "blargh").CombinedOutput()
	if err != nil {
		t.Logf("Output: %s", out)
		t.Fatal(err)
	}

	if string(out) != "llamas rock" {
		t.Fatalf("Unexpected output %q", out)
	}

	mt := &testingT{}

	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockWithPassthroughToLocalCommand(t *testing.T) {
	m, err := bintest.NewMock("echo")
	if err != nil {
		t.Fatal(err)
	}

	m.PassthroughToLocalCommand()
	m.Expect("hello", "world")

	out, err := exec.Command(m.Path, "hello", "world").CombinedOutput()
	if err != nil {
		t.Logf("Output: %s", out)
		t.Fatal(err)
	}

	if string(out) != "hello world\n" {
		t.Fatalf("Unexpected output %q", out)
	}

	mt := &testingT{}

	m.Check(mt)
}

func TestCallingMockWithExpectationsOfNumberOfCalls(t *testing.T) {
	var testCases = []struct {
		label string
		n     int
	}{
		{"Zero", 0},
		{"Once", 1},
		{"Twice", 2},
		{"Infinite", bintest.InfiniteTimes},
	}

	for _, tc := range testCases {
		t.Run(tc.label, func(t *testing.T) {
			m, err := bintest.NewMock("test")
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()

			m.Expect("test").Times(tc.n)
			var failures int

			for c := 0; c < tc.n; c++ {
				if _, err := exec.Command(m.Path, "test").CombinedOutput(); err != nil {
					failures++
				}
			}

			if failures > 0 {
				t.Fatalf("Expected 0 failures, got %d", failures)
			}

			if m.Check(t) == false {
				t.Errorf("Assertions should have passed")
			}
		})
	}
}
