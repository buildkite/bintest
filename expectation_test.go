package bintest

import (
	"reflect"
	"testing"

	"github.com/buildkite/bintest/v3/testutil"
)

func TestMatchExpectations(t *testing.T) {
	var exp = ExpectationSet{
		{name: "test", arguments: Arguments{"llamas", "rock"}, minCalls: 1, maxCalls: 5},
		{name: "test", arguments: Arguments{"llamas", "are", "ok"}, minCalls: 1, maxCalls: 5},
		{name: "blargh", arguments: Arguments{"alpacas", "too"}, minCalls: 1, maxCalls: 5},
	}

	match, err := exp.ForArguments("llamas", "are", "ok").Match()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(exp[1], match) {
		t.Fatalf("Got unexpected %#v", match)
	}
}

func TestMatchOverlappingExpectations(t *testing.T) {
	var exp = ExpectationSet{
		{name: "test", arguments: Arguments{"llamas", "rock"}, totalCalls: 1, minCalls: 1, maxCalls: 1},
		{name: "blargh", arguments: Arguments{"alpacas", "too"}, minCalls: 1, maxCalls: 5},
		{name: "test", arguments: Arguments{"llamas", "rock"}, minCalls: 1, maxCalls: 5},
	}

	match, err := exp.ForArguments("llamas", "rock").Match()
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(exp[2], match) {
		t.Fatalf("Got unexpected %#v", match)
	}
}

func TestExplainExpectationMatch(t *testing.T) {
	var exp = ExpectationSet{
		{name: "test", arguments: Arguments{"llamas", "rock"}, totalCalls: 1, minCalls: 1, maxCalls: 1},
		{name: "blargh", arguments: Arguments{"alpacas"}, minCalls: 1, maxCalls: 5},
		{name: "test", arguments: Arguments{"llamas", "rock"}, minCalls: 1, maxCalls: 5},
	}

	actual := exp.ForArguments("llamas", "ro").ClosestMatch().Explain()
	expected := `Argument #2 doesn't match: Differs at character 3, expected "ck", got ""`

	if actual != expected {
		t.Fatalf("Wrong explanation, got %s, wanted %s", actual, expected)
	}
}

func TestCheckIndividualExpectations(t *testing.T) {
	var match = []*Expectation{
		{name: "test", arguments: Arguments{"llamas", "rock"}, totalCalls: 1, minCalls: 1, maxCalls: 1},
		{name: "blargh", arguments: Arguments{"alpacas", "too"}, totalCalls: 1, minCalls: 1, maxCalls: 5},
		{name: "test", arguments: Arguments{"llamas", "rock"}, totalCalls: 10, minCalls: 1, maxCalls: InfiniteTimes},
	}

	for _, exp := range match {
		if !exp.Check(t) {
			t.Fatalf("Expected %s to match", exp)
		}
	}

	var notMatch = []*Expectation{
		{name: "test", arguments: Arguments{"llamas", "rock"}, totalCalls: 0, minCalls: 1, maxCalls: 1},
		{name: "blargh", arguments: Arguments{"alpacas", "too"}, totalCalls: 10, minCalls: 1, maxCalls: 5},
		{name: "test", arguments: Arguments{"llamas", "rock"}, totalCalls: 0, minCalls: 1},
	}

	for _, exp := range notMatch {
		if exp.Check(&testutil.TestingT{}) {
			t.Fatalf("Expected %s to NOT match", exp)
		}
	}
}

func TestExpectionWithStdin(t *testing.T) {
	testCases := []struct {
		label     string
		stdin     interface{}
		readStdin []byte
		expect    bool
	}{
		{"string match", "hello", []byte("hello"), true},
		{"string mismatch", "hello", []byte("world"), false},
		{"MatchPattern match", MatchPattern("lo$"), []byte("hello"), true},
		{"MatchPattern mismatch", MatchPattern("^lo"), []byte("hello"), false},
		{"MatchAny", MatchAny(), []byte("hello"), true},
		{"MatchAny nil readStdin", MatchAny(), nil, true},
		{"string match nil readStdin", "nope", nil, false},
	}
	for _, tc := range testCases {
		t.Run(tc.label, func(t *testing.T) {
			exp := Expectation{stdin: tc.stdin, readStdin: tc.readStdin}
			fakeT := &testutil.TestingT{}
			if exp.Check(fakeT) != tc.expect {
				t.Errorf("expected Check() to be %v for %s", tc.expect, tc.label)
			}
			expectedLogs := 0
			if tc.expect == false {
				expectedLogs = 1
			}
			if len(fakeT.Logs) != expectedLogs {
				t.Errorf("expected %d errors when Check() is %v, got %d", expectedLogs, tc.expect, len(fakeT.Logs))
			}
		})
	}
}
