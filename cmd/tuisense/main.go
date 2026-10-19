package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Phobby/TuiSense/internal/app"
	"github.com/Phobby/TuiSense/internal/system"
)

var version = "0.6.0"

func main() {
	mockFlag := flag.Bool("mock", false, "Launch TuiSense using the interactive mock DualSense controller")
	noTestDevicesFlag := flag.Bool("no-test-devices", false, "Hide UI/architecture test devices (mock Xbox controller)")
	configDirFlag := flag.String("config-dir", "", "Custom directory for settings.json (default: $XDG_CONFIG_HOME/tuisense)")
	noMockFlag := flag.Bool("no-mock", false, "Launch with real DualSense hardware detection only (disable mock fallback)")
	noMockFallbackFlag := flag.Bool("no-mock-fallback", false, "Alias for --no-mock")
	fpsFlag := flag.Int("fps", 30, "Target animation frame rate (15..120)")
	profileDirFlag := flag.String("profile-dir", "", "Custom directory for controller profiles (default: $XDG_CONFIG_HOME/tuisense/profiles)")
	printUdevFlag := flag.Bool("print-udev", false, "Print required Linux udev rules for rootless DualSense hidraw access and exit")
	versionFlag := flag.Bool("version", false, "Display the installed TuiSense version and exit")

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "TuiSense v%s — Terminal-native, capability-driven controller management\n\n", version)
		fmt.Fprintf(out, "Usage:\n")
		fmt.Fprintf(out, "  tuisense [options]\n\n")
		fmt.Fprintf(out, "Options:\n")
		fmt.Fprintf(out, "  --mock             Launch using the interactive mock DualSense controller\n")
		fmt.Fprintf(out, "  --no-mock          Launch with real hardware detection only (no mock fallback)\n")
		fmt.Fprintf(out, "  --no-test-devices  Hide the mock Xbox test controller from the device selector\n")
		fmt.Fprintf(out, "  --config-dir <p>   Custom directory for settings.json (mode, last device)\n")
		fmt.Fprintf(out, "  --fps <int>        Target animation and telemetry frame rate (default: 30)\n")
		fmt.Fprintf(out, "  --profile-dir <p>  Custom directory for saving/loading JSON profiles\n")
		fmt.Fprintf(out, "  --print-udev       Print required Linux udev rules and exit\n")
		fmt.Fprintf(out, "  --version          Display the installed application version and exit\n")
		fmt.Fprintf(out, "  -h, --help         Display this help message and exit\n\n")
		fmt.Fprintf(out, "Directories (XDG Base Directory Specification):\n")
		fmt.Fprintf(out, "  Config & Profiles: %s\n", system.ProfilesDir())
		fmt.Fprintf(out, "  Application Data:  %s\n", system.DataDir())
		fmt.Fprintf(out, "  Cache:             %s\n", system.CacheDir())
	}

	flag.Parse()

	if *versionFlag {
		fmt.Printf("TuiSense v%s (universal controller TUI · DualSense backend)\n", version)
		return
	}

	if *printUdevFlag {
		fmt.Print(system.UdevRulesContent)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	disableMockFallback := *noMockFlag || *noMockFallbackFlag

	application := app.New(app.Options{
		ForceMock:     *mockFlag,
		AllowMock:     !disableMockFallback,
		NoTestDevices: *noTestDevicesFlag,
		ProfileDir:    *profileDirFlag,
		ConfigDir:     *configDirFlag,
		TargetFPS:     *fpsFlag,
		Version:       version,
	})

	if err := application.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "tuisense error: %v\n", err)
		os.Exit(1)
	}
}
