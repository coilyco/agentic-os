package main

import (
	"os"
	"testing"
)

// A holder or daemon the tests start is this test binary run again with the
// verb, since os.Executable is what the daemon launches.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && (os.Args[1] == holdCommand || os.Args[1] == "daemon") {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
