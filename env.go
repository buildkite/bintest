package bintest

import (
	"fmt"
	"strings"
)

type env []string

type EnvMatchResult struct {
	IsMatch     bool
	MatchCount  int
	Explanation string
}

func (environ env) Match(expect ...string) (result EnvMatchResult) {
	for _, e := range expect {
		pair := strings.Split(e, "=")
		actual, ok := GetEnv(pair[0], environ)
		if !ok {
			result.Explanation = fmt.Sprintf("Expected env %s, %s wasn't set in environment", e, pair[0])
			result.IsMatch = false
			continue
		}
		if actual != pair[1] {
			result.Explanation = fmt.Sprintf("Expected env %s, got %q", e, actual)
			result.IsMatch = false
			continue
		}
		result.MatchCount++
	}
	if result.MatchCount == len(expect) {
		result.IsMatch = true
	}
	return result
}

// ExpectEnv asserts that certain environment vars/values exist, otherwise
// an error is reported to T and a matching error is returned (for Before)
// func ExpectEnv(t *testing.T, environ []string, expect ...string) error {
// 	for _, e := range expect {
// 		pair := strings.Split(e, "=")
// 		actual, ok := GetEnv(pair[0], environ)
// 		if !ok {
// 			err := fmt.Errorf("Expected %s, %s wasn't set in environment", e, pair[0])
// 			t.Error(err)
// 			return err
// 		}
// 		if actual != pair[1] {
// 			err := fmt.Errorf("Expected %s, got %q", e, actual)
// 			t.Error(err)
// 			return err
// 		}
// 	}
// 	return nil
// }

// GetEnv returns the value for a given env in the invocation
func GetEnv(key string, environ []string) (string, bool) {
	for _, e := range environ {
		pair := strings.Split(e, "=")
		if strings.ToUpper(pair[0]) == strings.ToUpper(key) {
			return pair[1], true
		}
	}
	return "", false
}
