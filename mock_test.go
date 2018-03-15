package bintest_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/buildkite/bintest"
	"github.com/fortytw2/leaktest"
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

func tearDown(t *testing.T) func() {
	leakTest := leaktest.Check(t)
	return func() {
		leakTest()
	}
}

func TestCallingMockWithStdErrExpected(t *testing.T) {
	defer tearDown(t)()

	m, err := bintest.NewMock("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	m.Expect("blargh").AndWriteToStderr("llamas").AndExitWith(0)

	out, err := exec.Command(m.Path, "blargh").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	mt := &testingT{}

	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed")
	}

	if string(out) != "llamas" {
		t.Fatalf("Expected llamas, got %q", out)
	}
}

func TestCallingMockWithStdOutExpected(t *testing.T) {
	defer tearDown(t)()

	m, err := bintest.NewMock("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	m.Expect("blargh").AndWriteToStdout("llamas").AndExitWith(0)

	out, err := exec.Command(m.Path, "blargh").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	mt := &testingT{}

	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed")
	}

	if string(out) != "llamas" {
		t.Fatalf("Expected llamas, got %q", out)

	}
}

func TestCallingMockWithNoExpectationsSet(t *testing.T) {
	defer tearDown(t)()

	m, err := bintest.NewMock("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

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
	defer tearDown(t)()

	m, err := bintest.NewMock("test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

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
	defer tearDown(t)()

	m, err := bintest.NewMock("echo")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	m.Expect("hello world").AndPassthroughToLocalCommand("echo")

	out, err := exec.Command(m.Path, "hello world").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	if m.Check(&testingT{}) == false {
		t.Errorf("Assertions should have passed")
	}

	expected := "hello world\n"

	if string(out) != expected {
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
			defer tearDown(t)()

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
	defer tearDown(t)()

	m, err := bintest.NewMock("echo")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

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

	if m.Check(&testingT{}) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockRequiresExpectations(t *testing.T) {
	defer tearDown(t)()

	m, err := bintest.NewMock("llamas")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	err = exec.Command(m.Path, "first", "call").Run()
	if err == nil {
		t.Fatal(err)
	}

	if m.Check(&testingT{}) == false {
		t.Errorf("Assertions should have failed")
	}
}

func TestMockIgnoringUnexpectedInvocations(t *testing.T) {
	defer tearDown(t)()

	m, err := bintest.NewMock("llamas")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	m.Expect("first", "call").Once()
	m.Expect("third", "call").Once()
	m.Expect("fifth", "call").Once()
	m.Expect("seventh", "call").NotCalled()

	m.Expect().WithAnyArguments().AtLeastOnce().AndExitWith(0)

	_ = exec.Command(m.Path, "first", "call").Run()
	_ = exec.Command(m.Path, "second", "call").Run()
	_ = exec.Command(m.Path, "third", "call").Run()
	_ = exec.Command(m.Path, "fourth", "call").Run()
	_ = exec.Command(m.Path, "fifth", "call").Run()

	if m.Check(&testingT{}) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockOptionally(t *testing.T) {
	defer tearDown(t)()

	m, err := bintest.NewMock("llamas")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	m.Expect("first", "call").Optionally()
	m.Expect("third", "call").Once()

	_ = exec.Command(m.Path, "first", "call").Run()
	_ = exec.Command(m.Path, "third", "call").Run()

	if m.Check(t) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockMultipleExpects(t *testing.T) {
	m, err := bintest.NewMock("llamas")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

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
	defer tearDown(t)()

	m, err := bintest.NewMock("llamas")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	m.Expect().AtLeastOnce()

	_ = exec.Command(m.Path).Run()
	_ = exec.Command(m.Path).Run()

	if m.Check(t) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockExpectWithMatcherFunc(t *testing.T) {
	defer tearDown(t)()

	m, err := bintest.NewMock("llamas")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

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
	defer tearDown(t)()

	m, err := bintest.NewMock("true")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	m.Before(func(i bintest.Invocation) error {
		if err := bintest.ExpectEnv(t, i.Env, `MY_CUSTOM_ENV=1`, `LLAMAS_ROCK=absolutely`); err != nil {
			return err
		}
		return nil
	}).
		Expect().
		AtLeastOnce().
		WithAnyArguments()

	cmd := exec.Command(m.Path)
	cmd.Env = append(os.Environ(), `MY_CUSTOM_ENV=1`, `LLAMAS_ROCK=absolutely`)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	if err = cmd.Run(); err != nil {
		t.Fatal(err)
	}

	if m.Check(t) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestMockParallelCommandsWithPassthrough(t *testing.T) {
	defer tearDown(t)()

	var wg sync.WaitGroup

	for i := 1; i < 3; i++ {
		tmpDir, err := ioutil.TempDir("", "parallel-mocks")
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
	defer tearDown(t)()

	m, err := bintest.NewMock("testmock")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	m.Expect("blargh").Exactly(1)

	cmd := exec.Command("./testmock", "blargh")
	cmd.Dir = filepath.Dir(m.Path)

	_, err = cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Expected no failures: %v", err)
	}

	mt := &testingT{}

	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed")
	}
}

func TestCallingMocksWithOverlappingExpectations(t *testing.T) {
	defer tearDown(t)()

	m, err := bintest.NewMock("testmock")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()

	m.Expect("llamas").
		Once().
		AndExitWith(0)

	m.Expect().
		WithAnyArguments().
		Exactly(2).
		AndPassthroughToLocalCommand("echo")

	cmd1 := exec.Command(m.Path, "llamas")
	_, err = cmd1.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	cmd2 := exec.Command(m.Path, "alpacas")
	_, err = cmd2.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	cmd3 := exec.Command(m.Path, "horses")
	_, err = cmd3.CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}

	mt := &testingT{}

	if m.Check(mt) == false {
		t.Errorf("Assertions should have passed")
	}
}
