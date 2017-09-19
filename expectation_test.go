package bintest

import (
	"fmt"
	"reflect"
	"testing"
)

func TestMatchExpectations(t *testing.T) {
	var exp = ExpectationSet{
		{name: "test", arguments: Arguments{"llamas", "rock"}, minCalls: 1, maxCalls: 5},
		{name: "test", arguments: Arguments{"llamas", "are", "ok"}, minCalls: 1, maxCalls: 5},
		{name: "blargh", arguments: Arguments{"alpacas", "too"}, minCalls: 1, maxCalls: 5},
	}

	match, err := exp.Match("llamas", "are", "ok")
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

	match, err := exp.Match("llamas", "rock")
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(exp[2], match) {
		t.Fatalf("Got unexpected %#v", match)
	}
}

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
		if exp.Check(&testingT{}) {
			t.Fatalf("Expected %s to NOT match", exp)
		}
	}
}
