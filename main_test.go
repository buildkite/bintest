package bintest_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lox/bintest"
	"github.com/lox/bintest/proxy"
	"github.com/lox/bintest/proxy/client"
)

func TestMain(m *testing.M) {
	flag.BoolVar(&bintest.Debug, "mock.debug", false, "Whether to show bintest debug")
	flag.BoolVar(&proxy.Debug, "proxy.debug", false, "Whether to show bintest proxy debug")
	flag.Parse()

	if strings.TrimSuffix(filepath.Base(os.Args[0]), `.exe`) != `bintest.test` {
		os.Exit(client.NewFromEnv().Run())
	}

	code := m.Run()
	os.Exit(code)
}
