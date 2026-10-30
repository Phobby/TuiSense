//go:build js && wasm

package system

// openNonblock: there are no device nodes in the browser (WebAssembly).
const openNonblock = 0
