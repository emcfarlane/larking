package main

import (
	"fmt"
	"io"
	"regexp"
	"runtime/pprof"
)

// testDeps is an implementation of the testing.testDeps interface,
// suitable for passing to testing.MainStart.
type testDeps struct {
	importPath string
}

var matchPat string
var matchRe *regexp.Regexp

func (testDeps) MatchString(pat, str string) (result bool, err error) {
	if matchRe == nil || matchPat != pat {
		matchPat = pat
		matchRe, err = regexp.Compile(matchPat)
		if err != nil {
			return
		}
	}
	return matchRe.MatchString(str), nil
}

func (testDeps) StartCPUProfile(w io.Writer) error {
	return pprof.StartCPUProfile(w)
}

func (testDeps) StopCPUProfile() {
	pprof.StopCPUProfile()
}

func (testDeps) WriteProfileTo(name string, w io.Writer, debug int) error {
	return pprof.Lookup(name).WriteTo(w, debug)
}

func (t testDeps) ImportPath() string {
	return t.importPath
}

func (t *testDeps) StartTestLog(w io.Writer) {}

func (testDeps) StopTestLog() error { return nil }

// SetPanicOnExit0 tells the os package whether to panic on os.Exit(0).
func (testDeps) SetPanicOnExit0(v bool) {
	// TODO?
	fmt.Println("SetPanicOnExit", v)
}
