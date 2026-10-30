package demo

import (
	"errors"
	"os/exec"
)

// ErrDisabled is returned by every host action the web demo refuses.
var ErrDisabled = errors.New("disabled in the web demo")

// CompatEnv is the Driver & Compatibility view of the web demo: a fixed
// Linux system on which nothing is installed, loaded or writable. It never
// inspects the server it runs on.
type CompatEnv struct{}

func (CompatEnv) GOOS() string                    { return "linux" }
func (CompatEnv) ModuleLoaded(string) bool        { return false }
func (CompatEnv) Writable(string) bool            { return false }
func (CompatEnv) Exists(string) bool              { return false }
func (CompatEnv) LookPath(string) (string, error) { return "", exec.ErrNotFound }

// Runner refuses every command: the web demo never runs a program.
type Runner struct{}

func (Runner) Run([]string) error { return ErrDisabled }
