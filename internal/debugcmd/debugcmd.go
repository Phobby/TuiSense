// Package debugcmd implements `tuisense debug …`: privacy-conscious hardware
// and input reports for identifying controllers without running a full
// diagnostic. Serial numbers and MAC addresses are never printed.
package debugcmd

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/padmodels"
	"github.com/Phobby/TuiSense/internal/support"
	"github.com/Phobby/TuiSense/internal/system"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

// Usage is printed for `tuisense debug` without arguments.
const Usage = `Usage:
  tuisense debug devices              List controllers and gamepads (USB IDs, drivers, identification)
  tuisense debug inputs [--watch N]   Show each gamepad's input capabilities and axis ranges;
                                      --watch N prints live input events for N seconds
                                      (move every stick, press every button and trigger)
  tuisense debug inputs --device /dev/input/eventX [--watch N]
  tuisense debug support-matrix       Print the controller support matrix (Markdown)

No serial numbers or MAC addresses are printed, so the output is safe to share.
`

// Run executes a debug subcommand and returns the process exit code.
func Run(args []string, out io.Writer, confirmed map[string]string) int {
	gamepad.SetOverrides(confirmed)
	if len(args) == 0 {
		fmt.Fprint(out, Usage)
		return 2
	}
	switch args[0] {
	case "devices":
		return devices(out)
	case "support-matrix":
		fmt.Fprint(out, support.Markdown(support.Matrix()))
		return 0
	case "inputs":
		fs := flag.NewFlagSet("inputs", flag.ContinueOnError)
		fs.SetOutput(out)
		watch := fs.Int("watch", 0, "print live events for N seconds")
		node := fs.String("device", "", "only this event node")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		return inputs(out, *node, time.Duration(*watch)*time.Second)
	}
	fmt.Fprint(out, Usage)
	return 2
}

func driver() *gamepad.Driver {
	return gamepad.NewDriver(append([]gamepad.Model{msigc20.Model()}, padmodels.Models()...)...)
}

func confidence(r gamepad.Resolution) string {
	switch r.Confidence {
	case gamepad.Identified:
		return "identified"
	case gamepad.Confirmed:
		return "confirmed by you"
	case gamepad.Probable:
		return "probable"
	}
	return "not identified"
}

func devices(out io.Writer) int {
	fmt.Fprintln(out, "TuiSense controller discovery")
	fmt.Fprintln(out, "=============================")
	ds, err := system.ScanDualSenseDevices()
	fmt.Fprintf(out, "\nSony DualSense (hidraw driver): %d found\n", len(ds))
	if err != nil {
		fmt.Fprintf(out, "  scan warning: %v\n", err)
	}
	for _, d := range ds {
		fmt.Fprintf(out, "  - %s  %04x:%04x  %s  %s\n", d.Model, d.VendorID, d.ProductID, d.Connection, d.HIDRawPath)
	}

	drv := driver()
	found, err := drv.Scan()
	fmt.Fprintf(out, "\nevdev gamepads: %d found\n", len(found))
	if err != nil {
		fmt.Fprintf(out, "  scan error: %v\n", err)
		return 1
	}
	if len(found) == 0 {
		fmt.Fprintln(out, "  (none) — is the controller plugged in? Check `lsusb` and the cable (some cables are charge-only).")
	}
	for _, f := range found {
		d := f.Dev
		fmt.Fprintf(out, "\n  %s — %q\n", d.EventPath, d.Name)
		fmt.Fprintf(out, "    input id       bus %04x  vendor %04x  product %04x  version %04x\n", d.Bus, d.Vendor, d.Product, d.Version)
		fmt.Fprintf(out, "    kernel driver  %s\n", nonEmpty(d.KernelDriver, "(unknown)"))
		if u := d.USB; u != nil {
			fmt.Fprintf(out, "    USB            %04x:%04x  bcdDevice %04x  %q / %q  %s Mbit/s  bus %03d dev %03d\n",
				u.VendorID, u.ProductID, u.BCDDevice, u.Manufacturer, u.Product, u.SpeedMbps, u.BusNum, u.DevNum)
		}
		fmt.Fprintf(out, "    rumble (FF)    %v\n", d.HasFF(evdev.FFRumble))
		fmt.Fprintf(out, "    hardware key   %s\n", d.HardwareKey())
		model := "generic gamepad"
		if m := f.Resolution.Model; m != nil {
			model = m.Static.DisplayName
		}
		fmt.Fprintf(out, "    model          %s (%s)", model, confidence(f.Resolution))
		if f.Resolution.Reason != "" {
			fmt.Fprintf(out, " — %s", f.Resolution.Reason)
		}
		fmt.Fprintln(out)
		if f.Resolution.Confidence < gamepad.Confirmed {
			fmt.Fprintln(out, "    tip            if this is your MSI Force GC20 V2, confirm it in TuiSense (Diagnostics › Connection)")
		}
	}
	all, _ := evdev.Scan("/sys", "/dev/input")
	others := 0
	for _, d := range all {
		if !d.IsGamepad() && d.Bus == evdev.BusUSB && (d.Caps.Abs.Has(evdev.AbsX) || d.Caps.Key.Has(evdev.BtnSouth)) {
			others++
		}
		if d.IsGamepad() && d.Virtual() && !drv.IncludeVirtual {
			fmt.Fprintf(out, "\nIgnored virtual gamepad %s %q (%s) — software-created, e.g. by Steam Input.\n", d.EventPath, d.Name, d.HardwareKey())
		}
	}
	if others > 0 {
		fmt.Fprintf(out, "\n%d other USB input node(s) with axes or buttons were ignored (mice, keyboards, tablets).\n", others)
	}
	return 0
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func inputs(out io.Writer, node string, watch time.Duration) int {
	found, err := driver().Scan()
	if err != nil {
		fmt.Fprintf(out, "scan error: %v\n", err)
		return 1
	}
	n := 0
	for _, f := range found {
		if node != "" && f.Dev.EventPath != node {
			continue
		}
		n++
		describeInputs(out, f)
		if watch > 0 {
			watchEvents(out, f, watch)
		}
	}
	if n == 0 {
		fmt.Fprintln(out, "No matching gamepad found. Run `tuisense debug devices` first.")
		return 1
	}
	return 0
}

func describeInputs(out io.Writer, f gamepad.Discovered) {
	d := f.Dev
	fmt.Fprintf(out, "\n%s — %q (%s, driver %s)\n", d.EventPath, d.Name, d.HardwareKey(), nonEmpty(d.KernelDriver, "?"))
	dev, err := evdev.Open(d.EventPath)
	abs := map[uint16]evdev.AbsInfo{}
	if err != nil {
		fmt.Fprintf(out, "  cannot open: %v (ranges unavailable)\n", err)
	} else {
		defer dev.Close()
		for _, c := range d.Caps.Abs.Set() {
			if ai, err := dev.AbsInfo(c); err == nil {
				abs[c] = ai
			}
		}
	}
	fmt.Fprintln(out, "  absolute axes:")
	codes := d.Caps.Abs.Set()
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
	for _, c := range codes {
		ai, ok := abs[c]
		if !ok {
			fmt.Fprintf(out, "    %-12s (range unknown)\n", evdev.AbsName(c))
			continue
		}
		fmt.Fprintf(out, "    %-12s min %7d  max %7d  now %7d  fuzz %4d  flat %5d\n", evdev.AbsName(c), ai.Minimum, ai.Maximum, ai.Value, ai.Fuzz, ai.Flat)
	}
	var keys []string
	for _, c := range d.Caps.Key.Set() {
		keys = append(keys, evdev.KeyName(c))
	}
	fmt.Fprintf(out, "  buttons (%d): %s\n", len(keys), strings.Join(keys, " "))
	var ffs []string
	for _, c := range d.Caps.FF.Set() {
		ffs = append(ffs, evdev.FFName(c))
	}
	fmt.Fprintf(out, "  force feedback: %s\n", nonEmpty(strings.Join(ffs, " "), "none"))

	m := gamepad.InferMapping(d, abs)
	fmt.Fprintf(out, "  TuiSense mapping (%s):\n", gamepad.LayoutLabel(m.Layout))
	names := [4]string{"left stick X", "left stick Y", "right stick X", "right stick Y"}
	for i, a := range m.Sticks {
		if a.Present {
			fmt.Fprintf(out, "    %-14s ← %s\n", names[i], evdev.AbsName(a.Code))
		} else {
			fmt.Fprintf(out, "    %-14s ← (not found)\n", names[i])
		}
	}
	switch {
	case !m.HasTriggers:
		fmt.Fprintln(out, "    triggers       ← (no trigger axes or trigger buttons identified)")
	case m.TriggerKind == device.TriggerDigital:
		fmt.Fprintln(out, "    triggers       ← digital BTN_TL2 / BTN_TR2")
	case m.TriggerKind == device.TriggerSharedAxis:
		fmt.Fprintf(out, "    triggers       ← one shared axis %s\n", evdev.AbsName(m.Triggers[0].Code))
	default:
		fmt.Fprintf(out, "    triggers       ← analog %s / %s\n", evdev.AbsName(m.Triggers[0].Code), evdev.AbsName(m.Triggers[1].Code))
	}
	fmt.Fprintf(out, "    d-pad          ← %s\n", map[bool]string{true: "ABS_HAT0X/ABS_HAT0Y", false: "buttons (if any)"}[m.Hat])
	for _, b := range m.Buttons {
		if b.Code != 0 {
			fmt.Fprintf(out, "    %-14s ← %s\n", b.Label, evdev.KeyName(b.Code))
		}
	}
	for _, note := range m.Notes {
		fmt.Fprintf(out, "    note: %s\n", note)
	}
}

func watchEvents(out io.Writer, f gamepad.Discovered, d time.Duration) {
	dev, err := evdev.Open(f.Dev.EventPath)
	if err != nil {
		fmt.Fprintf(out, "  cannot open for watching: %v\n", err)
		return
	}
	defer dev.Close()
	fmt.Fprintf(out, "  watching for %s — move every stick, press every button and trigger…\n", d)
	deadline := time.Now().Add(d)
	seen := map[string]bool{}
	for time.Now().Before(deadline) {
		evs, err := dev.ReadEvents(256)
		if err != nil {
			fmt.Fprintf(out, "  read error: %v\n", err)
			return
		}
		for _, ev := range evs {
			switch ev.Type {
			case evdev.EvKey:
				fmt.Fprintf(out, "    %s %-14s %d\n", ev.Time.Format("15:04:05.000"), evdev.KeyName(ev.Code), ev.Value)
				seen[evdev.KeyName(ev.Code)] = true
			case evdev.EvAbs:
				fmt.Fprintf(out, "    %s %-14s %d\n", ev.Time.Format("15:04:05.000"), evdev.AbsName(ev.Code), ev.Value)
				seen[evdev.AbsName(ev.Code)] = true
			case evdev.EvSyn:
				if ev.Code == evdev.SynDropped {
					fmt.Fprintln(out, "    (events dropped by the kernel buffer)")
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	var names []string
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	fmt.Fprintf(out, "  inputs that changed: %s\n", nonEmpty(strings.Join(names, " "), "none"))
}

// ExitWith runs and exits (used by main).
func ExitWith(args []string, confirmed map[string]string) {
	os.Exit(Run(args, os.Stdout, confirmed))
}
