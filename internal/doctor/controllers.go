package doctor

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers"
	"github.com/Phobby/TuiSense/internal/drivers/xinput"
	"github.com/Phobby/TuiSense/internal/system"
	"github.com/Phobby/TuiSense/internal/system/winhid"
	"github.com/Phobby/TuiSense/internal/version"
)

// Controller is the read-only diagnosis of one attached controller.
type Controller struct {
	Driver, Model, ModelID, Visual, RawName, IDs, Connection, Backend, Identity, Evidence, Input string
	Features                                                                                     []string
	Unavailable                                                                                  []string
}

// HIDEntry is one HID game controller as Windows lists it (no path).
type HIDEntry struct {
	IDs, Usage, Manufacturer, Product string
	XInput, Bluetooth                 bool
}

// Hardware is the detailed controller section of the report.
type Hardware struct {
	Backends    []string
	Controllers []Controller
	HID         []HIDEntry
	Errors      []string
}

// ScanHardware discovers and opens each controller through the built-in
// drivers, records what TuiSense would show, and closes it again. No setting,
// vendor command or rumble is sent; closing an XInput controller only sets
// its motors to off, exactly as TuiSense does on exit.
func ScanHardware(reg *device.Registry) Hardware {
	var h Hardware
	for _, drv := range reg.Drivers() {
		h.Backends = append(h.Backends, drv.ID())
		cands, err := drv.Discover()
		if err != nil {
			h.Errors = append(h.Errors, fmt.Sprintf("%s: %v", drv.ID(), err))
			continue
		}
		for _, c := range cands {
			dev, err := drv.Open(c, nil)
			if err != nil {
				h.Errors = append(h.Errors, fmt.Sprintf("%s: open failed: %v", drv.ID(), err))
				continue
			}
			h.Controllers = append(h.Controllers, describe(drv.ID(), dev))
			_ = dev.Close()
		}
	}
	if all, err := winhid.Enumerate(); err != nil {
		h.Errors = append(h.Errors, "HID listing: "+err.Error())
	} else {
		for _, d := range winhid.GameControllers(all) {
			h.HID = append(h.HID, HIDEntry{IDs: d.HardwareKey(), Usage: fmt.Sprintf("%02x:%02x", d.UsagePage, d.Usage),
				Manufacturer: d.Manufacturer, Product: d.Product, XInput: d.XInput(), Bluetooth: d.Bluetooth()})
		}
	}
	return h
}

func describe(driver string, dev device.Device) Controller {
	desc, info, caps := dev.Descriptor(), dev.Info(), dev.Capabilities()
	c := Controller{Driver: driver, Model: desc.DisplayName, ModelID: desc.ModelID, Visual: desc.VisualID, RawName: info.Name,
		Connection: string(info.Connection), Backend: caps.Spec(device.CapConnection).Backend,
		Identity: info.IdentityState, Evidence: info.Identification, Input: "readable"}
	if info.VendorID != 0 || info.ProductID != 0 {
		c.IDs = fmt.Sprintf("%04x:%04x", info.VendorID, info.ProductID)
		if info.IdentityKey != "" {
			c.IDs = info.IdentityKey
		}
	}
	if !caps.Supports(device.CapDiagnostics) {
		c.Input = "not available (" + info.PermissionHint + ")"
	}
	for id, s := range caps {
		switch {
		case s.Accessible():
			c.Features = append(c.Features, string(id))
		case s.Physical() == device.Present:
			c.Unavailable = append(c.Unavailable, fmt.Sprintf("%s (%s)", id, s.Requires))
		}
	}
	sort.Strings(c.Features)
	sort.Strings(c.Unavailable)
	return c
}

// WriteHardware prints the detailed section.
func WriteHardware(w io.Writer, goos string, h Hardware) {
	fmt.Fprintln(w, "\nController details")
	if goos != "windows" {
		fmt.Fprintf(w, "  Note: this is %s; the Windows-specific listings below are empty here.\n", goos)
	}
	fmt.Fprintf(w, "  Backends: %s\n", strings.Join(h.Backends, ", "))
	fmt.Fprintf(w, "  Controllers found: %d\n", len(h.Controllers))
	if len(h.Controllers) == 0 {
		fmt.Fprintln(w, "  No controller detected. Check the USB/Bluetooth connection and try again.")
	}
	for i, c := range h.Controllers {
		fmt.Fprintf(w, "  [%d] %s\n", i+1, c.Model)
		row := func(k, v string) {
			if v != "" {
				fmt.Fprintf(w, "      %-14s %s\n", k, v)
			}
		}
		row("Device name", c.RawName)
		row("IDs", c.IDs)
		row("Connection", c.Connection)
		row("Driver", c.Driver+" (backend "+c.Backend+")")
		row("Registry", c.ModelID)
		row("3D model", c.Visual)
		row("Identity", c.Identity)
		row("Evidence", c.Evidence)
		row("Input", c.Input)
		row("Available", strings.Join(c.Features, ", "))
		row("Present, n/a", strings.Join(c.Unavailable, ", "))
	}
	if len(h.HID) > 0 {
		fmt.Fprintln(w, "  HID game controllers listed by Windows:")
		for _, d := range h.HID {
			route := "HID only (not visible to XInput)"
			if d.XInput {
				route = "XInput-class"
			}
			conn := "USB"
			if d.Bluetooth {
				conn = "Bluetooth"
			}
			fmt.Fprintf(w, "    %s usage %s %s, %s, %q %q\n", d.IDs, d.Usage, conn, route, d.Manufacturer, d.Product)
		}
	}
	for _, e := range h.Errors {
		fmt.Fprintf(w, "  ! %s\n", e)
	}
}

// RunArgs runs `tuisense doctor [--windows] [--export]`.
func RunArgs(args []string, w io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(w)
	detail := fs.Bool("windows", false, "include controller, driver and identity details (designed for Windows troubleshooting)")
	export := fs.Bool("export", false, "also save the report as a privacy-redacted text file to share")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var b strings.Builder
	fmt.Fprintf(&b, "TuiSense doctor — %s, %s/%s, %s\n", version.Display(), runtime.GOOS, runtime.GOARCH, osVersion())
	fmt.Fprintf(&b, "Installation: %s\n\n", installMethod(System().Exe))
	_ = Run(&b)
	if *detail || *export {
		fmt.Fprintf(&b, "\nRuntime: XInput DLL %s, hid.dll %s\n", yesNo(xinput.Available()), yesNo(winhid.Available()))
		WriteHardware(&b, runtime.GOOS, ScanHardware(drivers.NewBuiltinRegistry(drivers.RegistryOptions{Hardware: true})))
	}
	report := Redact(b.String(), homeDir(), os.Getenv("USERNAME"), os.Getenv("USER"))
	fmt.Fprint(w, report)
	if *export {
		dir := filepath.Join(system.DataDir(), "reports")
		path := filepath.Join(dir, "tuisense-doctor-"+time.Now().Format("20060102-150405")+".txt")
		header := "TuiSense diagnostic report. Serial numbers, Bluetooth addresses, Windows device instance paths,\n" +
			"your user name and environment variables are not included. Review it before sharing.\n\n"
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(w, "\nCould not save the report: %v\n", err)
			return 1
		}
		if err := os.WriteFile(path, []byte(header+report), 0o644); err != nil {
			fmt.Fprintf(w, "\nCould not save the report: %v\n", err)
			return 1
		}
		fmt.Fprintf(w, "\nSaved a shareable report to %s\n", path)
	}
	return 0
}

func yesNo(b bool) string {
	if b {
		return "available"
	}
	return "not available"
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// Redact replaces the user's home directory and names in a report.
func Redact(s, home string, names ...string) string {
	if home != "" {
		s = strings.ReplaceAll(s, home, "~")
		s = strings.ReplaceAll(s, filepath.ToSlash(home), "~")
	}
	for _, n := range names {
		if len(n) >= 3 { // short names would redact ordinary words
			s = strings.ReplaceAll(s, n, "<user>")
		}
	}
	return s
}

// installMethod classifies how this executable was installed.
func installMethod(exe string) string {
	p := strings.ToLower(strings.ReplaceAll(exe, `\`, "/"))
	switch {
	case exe == "":
		return "unknown"
	case strings.Contains(p, "/programs/tuisense/bin/"):
		return "PowerShell installer (per-user)"
	case strings.Contains(p, "/.local/bin/"):
		return "install script (per-user)"
	case strings.Contains(p, "/windowsapps/") || strings.Contains(p, "/scoop/") || strings.Contains(p, "/winget/") || strings.Contains(p, "/chocolatey/"):
		return "package manager"
	case strings.Contains(p, "/go-build") || strings.Contains(p, "/temp/") || strings.Contains(p, "/tmp/"):
		return "temporary or development build"
	}
	return "portable (run from its own folder)"
}
