package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/Phobby/TuiSense/internal/app"
	"github.com/Phobby/TuiSense/internal/debugcmd"
	"github.com/Phobby/TuiSense/internal/doctor"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/system"
	"github.com/Phobby/TuiSense/internal/termcolor"
	"github.com/Phobby/TuiSense/internal/updatecmd"
	"github.com/Phobby/TuiSense/internal/version"
)

func usage() {
	out := flag.CommandLine.Output()
	fmt.Fprintf(out, "TuiSense %s — Your Controller. Your Terminal.\n\n", version.Display())
	fmt.Fprintf(out, "Usage:\n")
	fmt.Fprintf(out, "  tuisense [options]               Start the terminal UI\n")
	fmt.Fprintf(out, "  tuisense update [--check]        Update TuiSense itself (never controller firmware)\n")
	fmt.Fprintf(out, "  tuisense update --rollback       Restore the version replaced by the last update\n")
	fmt.Fprintf(out, "  tuisense doctor                  Check terminal, clipboard, audio, permissions and drivers\n")
	fmt.Fprintf(out, "  tuisense debug devices|inputs|support-matrix\n\n")
	fmt.Fprintf(out, "Options:\n")
	fmt.Fprintf(out, "  --mock             Launch using the interactive mock DualSense controller\n")
	fmt.Fprintf(out, "  --test-devices     Explicitly enable simulated test controllers\n")
	fmt.Fprintf(out, "  --web-demo         Browser demo mode: simulated controllers only, no hardware,\n")
	fmt.Fprintf(out, "                     updates, driver installation or other host access\n")
	fmt.Fprintf(out, "  --config-dir <p>   Custom directory for settings.json (mode, last device)\n")
	fmt.Fprintf(out, "  --profile-dir <p>  Custom directory for saving/loading JSON profiles\n")
	fmt.Fprintf(out, "  --fps <int>        Target animation and telemetry frame rate (default: 30)\n")
	fmt.Fprintf(out, "  --print-udev       Print the Linux udev rule for rootless DualSense access and exit\n")
	fmt.Fprintf(out, "  --version          Print the installed version and exit\n")
	fmt.Fprintf(out, "  -h, --help         Show this help\n\n")
	fmt.Fprintf(out, "Directories:\n")
	fmt.Fprintf(out, "  Settings & profiles: %s\n", system.ConfigDir())
	fmt.Fprintf(out, "  Data (reports, updates): %s\n", system.DataDir())
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "debug":
			prefs, _ := state.NewPrefsStore(system.ConfigDir()).Load()
			debugcmd.ExitWith(os.Args[2:], prefs.ConfirmedModels)
		case "update":
			os.Exit(updatecmd.Run(os.Args[2:], os.Stdout, version.String()))
		case "doctor":
			os.Exit(doctor.Run(os.Stdout))
		}
	}

	mockFlag := flag.Bool("mock", false, "Launch TuiSense using the interactive mock DualSense controller")
	testDevicesFlag := flag.Bool("test-devices", false, "Explicitly enable simulated architecture test devices")
	noTestDevicesFlag := flag.Bool("no-test-devices", false, "Hide UI/architecture test devices (simulated Xbox and DualSense)")
	configDirFlag := flag.String("config-dir", "", "Custom directory for settings.json (default: $XDG_CONFIG_HOME/tuisense)")
	noMockFlag := flag.Bool("no-mock", false, "Compatibility option: real hardware is already the default")
	noMockFallbackFlag := flag.Bool("no-mock-fallback", false, "Alias for --no-mock")
	fpsFlag := flag.Int("fps", 30, "Target animation frame rate (15..120)")
	profileDirFlag := flag.String("profile-dir", "", "Custom directory for controller profiles (default: $XDG_CONFIG_HOME/tuisense/profiles)")
	printUdevFlag := flag.Bool("print-udev", false, "Print required Linux udev rules for rootless DualSense hidraw access and exit")
	versionFlag := flag.Bool("version", false, "Display the installed TuiSense version and exit")
	webDemoFlag := flag.Bool("web-demo", false, "Run the browser demo: simulated controllers only, no hardware or host access")
	flag.Usage = usage
	flag.Parse()

	if *versionFlag {
		build := runtime.GOOS + "/" + runtime.GOARCH
		if c := version.ShortCommit(); c != "" {
			build += ", commit " + c
		}
		fmt.Printf("TuiSense %s (%s)\n", version.Display(), build)
		return
	}
	if *printUdevFlag {
		fmt.Print(system.UdevRulesContent)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *webDemoFlag {
		os.Exit(runWebDemo(ctx, *configDirFlag, *profileDirFlag, *fpsFlag))
	}

	disableMockFallback := *noMockFlag || *noMockFallbackFlag
	application := app.New(app.Options{
		ForceMock:     *mockFlag,
		TestDevices:   *testDevicesFlag,
		AllowMock:     *mockFlag && !disableMockFallback,
		NoTestDevices: *noTestDevicesFlag,
		ProfileDir:    *profileDirFlag,
		ConfigDir:     *configDirFlag,
		TargetFPS:     *fpsFlag,
		Version:       version.String(),
	})
	updatecmd.CleanupAfterStart()
	termcolor.Apply() // before Limoni detects the terminal's colour depth

	err := application.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "tuisense error: %v\n", err)
		os.Exit(1)
	}
	// A verified update staged during this session is installed now that the
	// UI has exited; the next start runs the new version.
	if msg := application.ApplyStagedUpdate(); msg != "" {
		fmt.Println(msg)
	}
}

// runWebDemo runs the normal interface on simulated controllers for the
// browser demo. The demo server starts it in an isolated session with a
// private temporary home; the directories default to that home.
func runWebDemo(ctx context.Context, configDir, profileDir string, fps int) int {
	if configDir == "" {
		configDir = system.ConfigDir()
	}
	if profileDir == "" {
		profileDir = filepath.Join(configDir, "profiles")
	}
	application := app.New(app.Options{
		WebDemo:    true,
		ConfigDir:  configDir,
		ProfileDir: profileDir,
		ReportDir:  filepath.Join(system.DataDir(), "reports"),
		TargetFPS:  fps,
		Version:    version.String(),
	})
	termcolor.Apply()
	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "tuisense error: %v\n", err)
		return 1
	}
	return 0
}
