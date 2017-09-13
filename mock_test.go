package bintest_test

import (
	"os/exec"
	"testing"

	"github.com/lox/bintest"
)

func TestCallingMockWithNoExpectationsSet(t *testing.T) {
	m, err := bintest.NewMock("test")
	if err != nil {
		t.Fatal(err)
	}

	_, err = exec.Command(m.Path, "blargh").CombinedOutput()
	if err == nil {
		t.Errorf("Expected a failure without any expectations set")
	}

	if m.Check(t) == false {
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

	if m.Check(t) == false {
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

	m.Check(t)
}
