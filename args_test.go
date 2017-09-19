package bintest_test

import (
	"testing"

	"github.com/lox/bintest"
)

func TestArgumentsThatDontMatch(t *testing.T) {
	var testCases = []struct {
		expected bintest.Arguments
		actual   []string
	}{
		{
			bintest.Arguments{"test", "llamas", "rock"},
			[]string{"test", "llamas", "alpacas"},
		},
		{
			bintest.Arguments{"test", "llamas"},
			[]string{"test", "llamas", "alpacas"},
		},
	}

	for _, test := range testCases {
		match, _ := test.expected.Match(test.actual...)
		if match {
			t.Fatalf("Expected [%s] and [%s] to NOT match",
				test.expected, bintest.FormatStrings(test.actual))
		}
	}
}

func TestArgumentsThatMatch(t *testing.T) {
	var testCases = []struct {
		expected bintest.Arguments
		actual   []string
	}{
		{
			bintest.Arguments{"test", "llamas", "rock"},
			[]string{"test", "llamas", "rock"},
		},
		{
			bintest.Arguments{"test", "llamas", bintest.MatchAny()},
			[]string{"test", "llamas", "rock"},
		},
	}

	for _, test := range testCases {
		match, _ := test.expected.Match(test.actual...)
		if !match {
			t.Fatalf("Expected [%s] and [%s] to match",
				test.expected, bintest.FormatStrings(test.actual))
		}
	}
}

func TestArgumentsToString(t *testing.T) {
	var testCases = []struct {
		args     bintest.Arguments
		expected string
	}{
		{
			bintest.Arguments{"test", "llamas", "rock"},
			`"test", "llamas", "rock"`,
		},
		{
			bintest.Arguments{"test", "llamas", bintest.MatchAny()},
			`"test", "llamas", bintest.MatchAny()`,
		},
	}

	for _, test := range testCases {
		actual := test.args.String()
		if actual != test.expected {
			t.Fatalf("Expected [%s], got [%s]", test.expected, actual)
		}
	}
}
