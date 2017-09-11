Binproxy
=========

Creates binaries (anything that golang compiles to) that proxy their invocation
back to the caller. Designed as a building block for mocking out external binaries
in tests.

Usage
-----

```go
// create a proxy for the git command that echos some debug
proxy, err := binproxy.New("git")
if err != nil {
	log.Fatal(err)
}

// call the proxy like a normal binary
go fmt.Println(exec.Command("git", "test", "arguments").CombinedOutput())

// handle invocations of the proxy binary
for call := range proxy.Ch {
  fmt.Fprintln(call.Stdout, "Llama party! 🎉")
	call.Exit(0)
}

// Llama party! 🎉
```

Testing
-------

The key thing that binproxy is designed for is mocking binaries in tests. Think
of it like mock's for binaries. Inspired by bats-mock and go-binmock.
