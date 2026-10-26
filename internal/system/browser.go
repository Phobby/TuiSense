package system

import (
	"context"
	"errors"
	"github.com/Phobby/TuiSense/internal/clipboard"
	"net/url"
	"os/exec"
	"runtime"
)

// OpenURL opens an HTTPS web page in the user's default browser. It only
// launches the platform's standard URL handler and never runs anything else.
func OpenURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("only https:// web pages can be opened")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		cmd = exec.Command("xdg-open", u.String())
	case "darwin":
		cmd = exec.Command("open", u.String())
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u.String())
	default:
		return errors.New("opening a browser is not supported on " + runtime.GOOS)
	}
	// Keep the browser's output away from the terminal UI.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// CopyText is the compatibility adapter for callers that require an acknowledged copy.
func CopyText(text string) error {
	out, err := clipboard.New().Copy(context.Background(), text)
	if err != nil {
		return err
	}
	if !out.Confirmed {
		return errors.New("terminal clipboard request sent; paste to verify")
	}
	return nil
}
