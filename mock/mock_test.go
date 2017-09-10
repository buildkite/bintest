package mock_test

import (
	"testing"

	"github.com/lox/go-binproxy/mock"
)

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
		match, descr := test.expected.Match(test.actual...)
		t.Log(descr)

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
		match, descr := test.expected.Match(test.actual...)
		t.Log(descr)

		if !match {
			t.Fatalf("Expected %v and %v to match", test.expected, test.actual)
		}
	}
}
