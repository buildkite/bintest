package bintest_test

import (
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/lox/bintest/proxy"
)

func TestMain(m *testing.M) {
	initialGoRoutines := runtime.NumGoroutine()
	code := m.Run()

	// stop the proxy server
	if err := proxy.StopServer(); err != nil {
		log.Fatal(err)
	}

	// check for leaking go routines
	if code == 0 && !testing.Short() {
		time.Sleep(time.Millisecond * 100)

		if runtime.NumGoroutine() > initialGoRoutines {
			log.Printf("There are %d go routines left running", runtime.NumGoroutine()-initialGoRoutines)
			log.Fatal(pprof.Lookup("goroutine").WriteTo(os.Stdout, 1))
		}
	}

	os.Exit(code)
}
