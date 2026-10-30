//go:build !(js && wasm)

package system

import "syscall"

// openNonblock opens device nodes without blocking on them.
const openNonblock = syscall.O_NONBLOCK
