//go:build js && wasm

package app

import (
	"io"
	"syscall/js"
)

// terminalOutput writes to the browser terminal through the same bridge
// Limoni's WebAssembly backend renders through.
func terminalOutput() io.Writer { return jsTerminal{} }

type jsTerminal struct{}

func (jsTerminal) Write(p []byte) (int, error) {
	out := js.Global().Get("__limoni_output")
	if out.Type() != js.TypeFunction {
		return 0, io.ErrClosedPipe
	}
	out.Invoke(string(p))
	return len(p), nil
}
