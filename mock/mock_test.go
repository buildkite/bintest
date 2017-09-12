package mock_test

import (
	"os/exec"
	"testing"

	"github.com/lox/binproxy/mock"
)

func TestCallingMockWithNoExpectationsSet(t *testing.T) {
	m, err := mock.New("test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = exec.Command(m.Path, "blargh").CombinedOutput()
	if err == nil {
		t.Errorf("Expected a failure without any expectations set")
	}

	if m.AssertExpectations(t) == false {
		t.Errorf("Assertions should have passed (there were none)")
	}
}

func TestCallingMockWithExpectationsSet(t *testing.T) {
	m, err := mock.New("test")
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

	if m.AssertExpectations(t) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestArgumentsThatDontMatch(t *testing.T) {
	var testCases = []struct {
		expected mock.Arguments
		actual   []string
	}{
		{
			mock.Arguments{"test", "llamas", "rock"},
			[]string{"test", "llamas", "alpacas"},
		},
		{
			mock.Arguments{"test", "llamas"},
			[]string{"test", "llamas", "alpacas"},
		},
	}

	for _, test := range testCases {
		match, _ := test.expected.Match(test.actual...)
		if match {
			t.Fatalf("Expected %v and %v to not match", test.expected, test.actual)
		}
	}
}

func TestArgumentsThatMatch(t *testing.T) {
	var testCases = []struct {
		expected mock.Arguments
		actual   []string
	}{
		{
			mock.Arguments{"test", "llamas", "rock"},
			[]string{"test", "llamas", "rock"},
		},
		{
			mock.Arguments{"test", "llamas", mock.MatchAny()},
			[]string{"test", "llamas", "rock"},
		},
	}

	for _, test := range testCases {
		match, _ := test.expected.Match(test.actual...)
		if !match {
			t.Fatalf("Expected %v and %v to match", test.expected, test.actual)
		}
	}
}
