package compat

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// SystemEnv reads the real system. It only inspects files and PATH.
type SystemEnv struct{}

func (SystemEnv) GOOS() string { return runtime.GOOS }

// ModuleLoaded reports whether a kernel module is loaded (/sys/module/<name>).
func (SystemEnv) ModuleLoaded(name string) bool {
	_, err := os.Stat(filepath.Join("/sys/module", name))
	return err == nil
}

func (SystemEnv) Writable(path string) bool { return writable(path) }

func (SystemEnv) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (SystemEnv) LookPath(name string) (string, error) { return exec.LookPath(name) }

// ExecRunner runs argument vectors directly (no shell, no environment expansion).
type ExecRunner struct{}

func (ExecRunner) Run(argv []string) error {
	if len(argv) == 0 {
		return errUnknownCommand
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = nil
	return cmd.Run()
}
