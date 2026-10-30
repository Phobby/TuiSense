package app

import (
	"context"
	"errors"

	"github.com/Phobby/TuiSense/internal/clipboard"
	"github.com/Phobby/TuiSense/internal/demo"
	"github.com/Phobby/TuiSense/internal/firmware"
)

// webDemoDisabled is shown where the web demo refuses a host action.
const webDemoDisabled = "Not available in the web demo: it uses simulated controllers and never changes the server."

// webDemoOptions confines a web-demo run. The restrictions are enforced here
// and in the backends, not only by hiding menu items:
//   - the registry holds only simulated controllers (no hardware driver can
//     enumerate or open a device, so no HID, USB, Bluetooth or firmware access);
//   - no audio backend (simulated devices use in-memory audio);
//   - no firmware metadata provider (no network check);
//   - Driver & Compatibility reads a fixed, simulated system and its runner
//     refuses every command (no driver installation, no pkexec);
//   - links are never opened (no browser or other program is started);
//   - the clipboard is the browser terminal's (OSC 52), never a host tool.
//
// Application updates and suspend (Ctrl+Z) are disabled in New and Run.
func webDemoOptions(opts Options) Options {
	opts.ForceMock, opts.AllowMock, opts.TestDevices, opts.NoTestDevices = false, false, false, true
	if opts.Registry == nil {
		opts.Registry = demo.NewRegistry()
	}
	opts.AudioBackend = nil
	opts.FirmwareProviders = []firmware.Provider{}
	opts.CompatEnv, opts.CompatRunner = demo.CompatEnv{}, demo.Runner{}
	opts.OpenURL = func(string) error { return errors.New(webDemoDisabled) }
	if opts.Clipboard == nil && opts.CopyText == nil {
		opts.Clipboard = terminalClipboard()
	}
	opts.UpdateSource = nil
	return opts
}

// terminalClipboard copies through the browser terminal with OSC 52, which
// the web demo's xterm.js frontend forwards to the visitor's clipboard.
func terminalClipboard() clipboard.ClipboardService {
	return &clipboard.Service{
		GOOS: "webdemo", // no native clipboard tools are ever looked up
		Getenv: func(k string) string {
			switch k {
			case "TUISENSE_OSC52":
				return "1"
			case "TERM":
				return "xterm-256color"
			}
			return ""
		},
		LookPath: func(string) (string, error) { return "", demo.ErrDisabled },
		Run:      func(context.Context, string, []string, string) error { return demo.ErrDisabled },
		Terminal: terminalOutput(),
	}
}
