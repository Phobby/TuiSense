// Package compat checks whether the connected controller has the operating
// system drivers and permissions its features need, and offers safe,
// user-approved remedies.
//
// Safety rules (enforced here, not only in the UI):
//   - Nothing is installed automatically. Analyze only reads the system.
//   - Only actions on a fixed allowlist can be executed, only after explicit
//     confirmation, through fixed argument vectors (never a shell), and the
//     privileged step uses polkit's pkexec so the TUI itself never runs as root.
//   - Kernel modules, out-of-tree drivers and module blacklists are never
//     changed by TuiSense; they are offered as reviewed manual instructions.
//   - Existing, working drivers are never replaced.
package compat

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/Phobby/TuiSense/internal/device"
)

// Level grades a check.
type Level string

const (
	LevelOK      Level = "ok"
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelMissing Level = "missing"
)

// Check is one actionable finding about the connected controller.
type Check struct {
	ID      string
	Title   string
	Level   Level
	Detail  string // static, translatable text
	Value   string // variable part shown after Detail (driver names, paths)
	Affects []device.Capability
	// ActionID links a remedy, when one exists.
	ActionID string
}

// Action is a remedy. Executable actions run fixed commands after explicit
// confirmation; manual ones are instructions the user runs themselves.
type Action struct {
	ID          string
	Title       string
	Explanation string
	// Changes lists every system change the action makes.
	Changes []string
	// Commands are argument vectors run in order (no shell).
	Commands   [][]string
	Privileged bool
	// Manual actions are never executed by TuiSense.
	Manual       bool
	Instructions []string
	Rollback     []string
	// Reconnect: the controller must be reconnected afterwards.
	Reconnect bool
	Links     []string
}

// Report is the Driver & Compatibility summary for one controller.
type Report struct {
	Controller string
	OS         string
	Connection string
	Backend    string // TuiSense backend / OS driver in use
	Input      string // "Working", "No permission", …
	Supported  bool   // false: TuiSense has no hardware backend on this OS
	Checks     []Check
	Actions    []Action
}

// Action returns the action with id.
func (r Report) Action(id string) (Action, bool) {
	for _, a := range r.Actions {
		if a.ID == id {
			return a, true
		}
	}
	return Action{}, false
}

// Input is what Analyze needs about the active controller.
type Input struct {
	Desc      device.Descriptor
	Caps      device.CapabilitySet
	Info      device.Info
	Simulated bool
}

// Env is the read-only view of the system Analyze uses (faked in tests).
type Env interface {
	GOOS() string
	ModuleLoaded(name string) bool
	Writable(path string) bool
	Exists(path string) bool
	LookPath(name string) (string, error)
}

// Analyze inspects the system for the controller. It never changes anything.
func Analyze(env Env, in Input) Report {
	r := Report{
		Controller: in.Desc.DisplayName,
		OS:         osName(env.GOOS()),
		Connection: string(in.Info.Connection),
		Supported:  true,
	}
	if in.Simulated {
		r.Backend = "simulated"
		r.Input = "Simulated"
		r.Checks = append(r.Checks, Check{ID: "simulated", Title: "Simulated device", Level: LevelInfo,
			Detail: "This is a TuiSense mock or test device; no driver or permission is involved."})
		return r
	}
	switch env.GOOS() {
	case "linux":
	case "windows":
		windowsChecks(in, &r)
		return r
	case "darwin":
		macChecks(in, &r)
		return r
	default:
		r.Supported = false
		r.Checks = append(r.Checks, Check{ID: "platform", Title: "Hardware backends", Level: LevelInfo,
			Detail: "TuiSense's controller backends are implemented for Linux (and XInput on Windows). On this system the gallery, profiles and reports work, but no controller is read."})
		return r
	}
	linuxChecks(env, in, &r)
	return r
}

func osName(goos string) string {
	switch goos {
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	}
	return goos
}

// Current returns the host OS identifier (overridable in tests through Env).
func Current() string { return runtime.GOOS }

func has(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Errors returned when executing actions.
var (
	ErrNotConfirmed   = errors.New("the action was not confirmed")
	ErrNotExecutable  = errors.New("this action is manual; TuiSense never runs it")
	ErrUntrusted      = errors.New("the action is not on TuiSense's allowlist")
	ErrNoPolkit       = errors.New("pkexec (polkit) is not available; follow the manual instructions")
	ErrVerifyFailed   = errors.New("the change was applied but could not be verified")
	ErrActionChanged  = errors.New("the action differs from the reviewed one")
	errUnknownCommand = errors.New("unexpected command")
)

// Runner executes one argument vector without a shell.
type Runner interface {
	Run(argv []string) error
}

// Execute runs an executable action after confirmation. The action must be
// exactly one of the allowlisted actions TuiSense generates; every command
// is validated against that allowlist before anything runs.
func Execute(env Env, run Runner, a Action, confirmed bool, verify func() bool) error {
	if !confirmed {
		return ErrNotConfirmed
	}
	if a.Manual {
		return ErrNotExecutable
	}
	want, ok := allowlist[a.ID]
	if !ok {
		return ErrUntrusted
	}
	if a.Privileged {
		if _, err := env.LookPath("pkexec"); err != nil {
			return ErrNoPolkit
		}
	}
	if len(a.Commands) != len(want) {
		return ErrActionChanged
	}
	for i, argv := range a.Commands {
		if !want[i](argv) {
			return fmt.Errorf("%w: %s", ErrActionChanged, strings.Join(argv, " "))
		}
	}
	for _, argv := range a.Commands {
		if err := run.Run(argv); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
		}
	}
	if verify != nil && !verify() {
		return ErrVerifyFailed
	}
	return nil
}

// windowsBackendHID is the identity-only backend of HID game controllers
// (kept as a literal: compat must not import the driver packages).
const windowsBackendHID = "windows-hid-identity"

func macChecks(in Input, r *Report) {
	r.Backend = "iokit-hid"
	r.Input = "Working"
	if in.Info.PermissionDenied {
		r.Input = "Not available"
		r.Checks = append(r.Checks, Check{ID: "mac_input_monitoring", Title: "Controller access", Level: LevelWarning,
			Detail:  in.Info.PermissionHint,
			Affects: []device.Capability{device.CapSticks, device.CapDiagnostics, device.CapHaptics}})
	}
	r.Checks = append(r.Checks, Check{ID: "platform", Title: "Hardware backends", Level: LevelInfo,
		Detail: "On macOS TuiSense reads the DualSense and DualSense Edge through Apple's IOKit HID interface (USB and Bluetooth; experimental), with macOS' built-in support and no extra driver. Other controllers are not read on macOS yet."})
}

func windowsChecks(in Input, r *Report) {
	r.Backend = in.Caps.Spec(device.CapConnection).Backend
	r.Input = "Working"
	if r.Backend == windowsBackendHID {
		r.Input = "Not available"
		r.Checks = append(r.Checks, Check{ID: "windows_hid_input", Title: "Controller input", Level: LevelWarning,
			Detail:  "Controller detected, but input access is unavailable. Windows lists it as a HID (DirectInput-style) game controller, and TuiSense reads controller input on Windows only through XInput so far. If the controller has an input-mode switch or Mode button, select its XInput mode (on the MSI Force GC20 V2: Default mode, mode LEDs 1+2), then reconnect it. No driver needs to be installed.",
			Affects: []device.Capability{device.CapSticks, device.CapDiagnostics, device.CapHaptics}})
	}
	r.Checks = append(r.Checks, Check{ID: "platform", Title: "Hardware backends", Level: LevelInfo,
		Detail: "On Windows TuiSense reads XInput controllers through Microsoft's XInput API (input, rumble, battery; experimental) with Windows' built-in drivers. The model is named only when Windows' HID listing identifies it exactly; Xbox-compatible input alone never implies a specific manufacturer. DualSense, DualShock 4 and Switch Pro are recognised but have no Windows input backend yet."})
}
