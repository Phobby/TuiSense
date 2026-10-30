//go:build !(js && wasm)

package app

import (
	"io"
	"os"
)

// terminalOutput is where OSC 52 clipboard requests are written: the
// terminal TuiSense runs in.
func terminalOutput() io.Writer { return os.Stdout }
