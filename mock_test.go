package bintest_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/buildkite/bintest/v3"
	"github.com/buildkite/bintest/v3/testutil"
	"github.com/fortytw2/leaktest"
)

func TestCallingMockWithStdinExpectedString(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "test")
	defer close()

	m.Expect().WithStdin("the input")

	cmd := exec.Command(m.Path)
	cmd.Stdin = strings.NewReader("the input")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	mt := &testutil.TestingT{}
	if m.Check(mt) == false {
		t.Error("Mock.Check() failed")
	}
	mt.Copy(t)
}

func TestCallingMockWithStdinExpectedStringFailing(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "test")
	defer close()

	m.Expect().WithStdin("the expected")

	cmd := exec.Command(m.Path)
	cmd.Stdin = strings.NewReader("the unexpected")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	mt := &testutil.TestingT{}
	if m.Check(mt) == true {
		t.Error("Mock.Check() should have failed, but didn't")
	}
	if s := strings.Join(mt.Errors, "\n"); s != `Not all expectations were met (0 out of 1)` {
		t.Errorf("Errors: %q", s)
	}
	if s := strings.Join(mt.Logs, "\n"); s != `Expected stdin "the expected", got "the unexpected"` {
		t.Errorf("Logs: %q", s)
	}
}

func TestCallingMockWithStdinExpectedMatcher(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "test")
	defer close()

	m.Expect().WithStdin(bintest.MatchPattern("^the"))

	cmd := exec.Command(m.Path)
	cmd.Stdin = strings.NewReader("the input")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	mt := &testutil.TestingT{}
	if m.Check(mt) == false {
		t.Error("Mock.Check() failed")
	}
	mt.Copy(t)
}

func TestCallingMockWithStdinExpectedAndPassthrough(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "tr")
	defer close()

	m.PassthroughToLocalCommand()
	m.Expect("hs", "HS").WithStdin("hello stdin")

	cmd := exec.Command(m.Path, "hs", "HS")
	cmd.Stdin = strings.NewReader("hello stdin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	if expected, actual := "Hello Stdin", string(out); expected != actual {
		t.Errorf("Expected stdout %q, got %q", expected, actual)
	}

	mt := &testutil.TestingT{}
	if m.Check(mt) == false {
		t.Error("Mock.Check() failed")
	}
	mt.Copy(t)
}

func TestCallingMockWithStderrExpected(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "test")
	defer close()

	m.Expect("blargh").AndWriteToStderr("llamas").AndExitWith(0)

	out, err := exec.Command(m.Path, "blargh").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	mt := &testutil.TestingT{}
	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed")
	}
	if string(out) != "llamas" {
		t.Fatalf("Expected llamas, got %q", out)
	}
}

func TestCallingMockWithStdoutExpected(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "test")
	defer close()

	m.Expect("blargh").AndWriteToStdout("llamas").AndExitWith(0)

	out, err := exec.Command(m.Path, "blargh").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	mt := &testutil.TestingT{}
	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed")
	}
	if string(out) != "llamas" {
		t.Fatalf("Expected llamas, got %q", out)
	}
}

func TestCallingMockWithNoExpectationsSet(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "test")
	defer close()

	_, err := exec.Command(m.Path, "blargh").CombinedOutput()
	if err == nil {
		t.Errorf("Expected a failure without any expectations set")
	}

	mt := &testutil.TestingT{}
	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed (there were none)")
	}
}

func TestCallingMockWithExpectationsSet(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "test")
	defer close()

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

	mt := &testutil.TestingT{}
	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockWithPassthroughToLocalCommand(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "echo")
	defer close()

	m.PassthroughToLocalCommand()
	m.Expect("hello world")

	out, err := exec.Command(m.Path, "hello world").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	if m.Check(&testutil.TestingT{}) == false {
		t.Errorf("Assertions should have passed")
	}
	if expected := "hello world\n"; string(out) != expected {
		t.Fatalf("Expected %q, got %q", expected, out)
	}
}

func TestCallingMockWithExpectationsOfNumberOfCalls(t *testing.T) {
	var testCases = []struct {
		label    string
		n        int
		min, max int
	}{
		{"Zero", 0, 0, 0},
		{"Once", 1, 1, 1},
		{"Twice", 2, 2, 2},
		{"Infinite", 10, 10, bintest.InfiniteTimes},
		{"MinInfinite", 10, bintest.InfiniteTimes, bintest.InfiniteTimes},
	}

	for _, tc := range testCases {
		t.Run(tc.label, func(t *testing.T) {
			defer leaktest.Check(t)()

			m, err := bintest.NewMock("test")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := m.Close(); err != nil {
					t.Error(err)
				}
			}()

			m.Expect("test").Min(tc.min).Max(tc.max)
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

func TestMockWithCallFunc(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "echo")
	defer close()

	m.Expect("hello", "world").AndCallFunc(func(c *bintest.Call) {
		if !reflect.DeepEqual(c.Args[1:], []string{"hello", "world"}) {
			t.Errorf("Unexpected args: %v", c.Args)
		}
		fmt.Fprintf(c.Stdout, "hello world\n")
		c.Exit(0)
	})

	out, err := exec.Command(m.Path, "hello", "world").CombinedOutput()
	if err != nil {
		t.Logf("Output: %s", out)
		t.Fatal(err)
	}

	if string(out) != "hello world\n" {
		t.Fatalf("Unexpected output %q", out)
	}
	if m.Check(&testutil.TestingT{}) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockRequiresExpectations(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "llamas")
	defer close()

	if err := exec.Command(m.Path, "first", "call").Run(); err == nil {
		t.Fatal(err)
	}

	if m.Check(&testutil.TestingT{}) == false {
		t.Errorf("Assertions should have failed")
	}
}

func TestMockIgnoringUnexpectedInvocations(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "llamas")
	defer close()

	m.IgnoreUnexpectedInvocations()
	m.Expect("first", "call").Once()
	m.Expect("third", "call").Once()
	m.Expect("fifth", "call").Once()
	m.Expect("seventh", "call").NotCalled()

	_ = exec.Command(m.Path, "first", "call").Run()
	_ = exec.Command(m.Path, "second", "call").Run()
	_ = exec.Command(m.Path, "third", "call").Run()
	_ = exec.Command(m.Path, "fourth", "call").Run()
	_ = exec.Command(m.Path, "fifth", "call").Run()

	if m.Check(&testutil.TestingT{}) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockOptionally(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "llamas")
	defer close()

	m.Expect("first", "call").Optionally()
	m.Expect("third", "call").Once()

	_ = exec.Command(m.Path, "first", "call").Run()
	_ = exec.Command(m.Path, "third", "call").Run()

	if m.Check(t) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockMultipleExpects(t *testing.T) {
	m, close := mustMock(t, "llamas")
	defer close()

	m.Expect("first", "call")
	m.Expect("first", "call")
	m.Expect("first", "call")

	_ = exec.Command(m.Path, "first", "call").Run()
	_ = exec.Command(m.Path, "first", "call").Run()
	_ = exec.Command(m.Path, "first", "call").Run()

	if m.Check(t) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockExpectWithNoArguments(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "llamas")
	defer close()

	m.Expect().AtLeastOnce()

	_ = exec.Command(m.Path).Run()
	_ = exec.Command(m.Path).Run()

	if m.Check(t) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockExpectWithMatcherFunc(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "llamas")
	defer close()

	m.Expect().AtLeastOnce().WithMatcherFunc(func(arg ...string) bintest.ArgumentsMatchResult {
		return bintest.ArgumentsMatchResult{
			IsMatch:    true,
			MatchCount: len(arg),
		}
	})

	_ = exec.Command(m.Path, "x", "y").Run()
	_ = exec.Command(m.Path, "x").Run()
	_ = exec.Command(m.Path).Run()

	if m.Check(t) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockExpectWithBefore(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "true")
	defer close()

	m.PassthroughToLocalCommand().Before(func(i bintest.Invocation) error {
		if err := bintest.ExpectEnv(t, i.Env, `MY_CUSTOM_ENV=1`, `LLAMAS_ROCK=absolutely`); err != nil {
			return err
		}
		return nil
	})

	m.Expect().AtLeastOnce().WithAnyArguments()

	cmd := exec.Command(m.Path)
	cmd.Env = append(os.Environ(), `MY_CUSTOM_ENV=1`, `LLAMAS_ROCK=absolutely`)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if m.Check(t) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockParallelCommandsWithPassthrough(t *testing.T) {
	defer leaktest.Check(t)()

	var wg sync.WaitGroup

	for i := 1; i < 3; i++ {
		tmpDir, err := os.MkdirTemp("", "parallel-mocks")
		if err != nil {
			t.Fatal(err)
		}

		m, err := bintest.NewMock(filepath.Join(tmpDir, "sleep"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := m.Close(); err != nil {
				t.Error(err)
			}
		}()

		m.Expect(fmt.Sprintf("%d", i)).Exactly(1).AndPassthroughToLocalCommand("sleep")

		wg.Add(1)
		go func(path string, i int) {
			defer wg.Done()

			_, err := exec.Command(path, fmt.Sprintf("%d", i)).CombinedOutput()
			if err != nil {
				t.Errorf(err.Error())
			}

			if !m.Check(t) {
				t.Errorf("Assertions should have passed")
			}
		}(m.Path, i)
	}

	wg.Wait()
}

func TestCallingMockWithRelativePath(t *testing.T) {
	defer leaktest.Check(t)()
	m, close := mustMock(t, "testmock")
	defer close()

	m.Expect("blargh").Exactly(1)

	cmd := exec.Command("./testmock", "blargh")
	cmd.Dir = filepath.Dir(m.Path)

	_, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Expected no failures: %v", err)
	}

	mt := &testutil.TestingT{}

	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed")
	}
}

func mustMock(t *testing.T, name string) (*bintest.Mock, func()) {
	m, err := bintest.NewMock(name)
	if err != nil {
		t.Fatal(err)
	}
	return m, func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}
}
