//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "tuisense-webd runs on Linux only (it needs Linux pseudo-terminals, namespaces and Landlock); use the Docker image.")
	os.Exit(1)
}
