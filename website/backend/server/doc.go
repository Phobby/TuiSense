// Package server is the TuiSense web demo backend: it runs the real
// TuiSense binary (tuisense --web-demo) in an isolated pseudo-terminal per
// visitor and streams it to the browser terminal over a WebSocket.
//
// Each visitor gets a separate process with a private temporary home,
// started directly (never through a shell) by a sandbox helper that drops
// privileges, applies resource limits, and confines the process with Linux
// namespaces and Landlock where the kernel allows it. See
// docs/WEB_DEMO.md for the architecture and the security model.
//
// The backend runs on Linux only.
package server
