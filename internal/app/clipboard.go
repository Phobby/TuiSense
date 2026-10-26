package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Phobby/TuiSense/internal/clipboard"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/thebanri/limoni"
)

type copyCompletion struct {
	outcome  clipboard.Outcome
	err      error
	fallback string
}

// CopyText runs slow platform utilities off the render goroutine. A single
// in-flight operation prevents notification floods and preserves click order.
func (a *Application) CopyText(text string) {
	if a.copying {
		return
	}
	a.copying = true
	cp := a.Opts.Clipboard
	if cp == nil {
		cp = clipboard.New()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		var r copyCompletion
		if a.Opts.CopyText != nil {
			r.err = a.Opts.CopyText(text)
			r.outcome = clipboard.Outcome{Backend: "custom", Confirmed: r.err == nil}
		} else {
			r.outcome, r.err = cp.Copy(ctx, text)
		}
		if r.err != nil {
			// An exact, private UTF-8 file is accessible to keyboard and screen-reader
			// users even when the terminal/desktop clipboard is blocked.
			f, err := os.CreateTemp("", "tuisense-copy-*.txt")
			if err == nil {
				_, err = f.WriteString(text)
				closeErr := f.Close()
				if err == nil {
					err = closeErr
				}
				if err == nil {
					r.fallback = f.Name()
				} else {
					_ = os.Remove(f.Name())
				}
			}
			if err != nil {
				r.err = fmt.Errorf("%w; fallback file: %v", r.err, err)
			}
		}
		a.copyResults <- r
		limoni.Wakeup()
	}()
}

func (a *Application) drainCopies() {
	select {
	case r := <-a.copyResults:
		a.copying = false
		s := a.State
		switch {
		case r.err != nil:
			msg := i18n.Tf("Clipboard unavailable: %v", r.err)
			if r.fallback != "" {
				s.CopyFallbackPath = r.fallback
				s.ShowCopyFallback = true
				msg += "\n" + i18n.Tf("Exact text saved to %s. Open this UTF-8 file to copy it; delete it after use.", r.fallback)
			}
			s.SetError(i18n.T("Clipboard"), msg)
		case r.outcome.Confirmed:
			s.CopyStatus = i18n.T("Copied to clipboard")
			s.CopyStatusUntil = time.Now().Add(2 * time.Second)
		default:
			s.CopyStatus = i18n.T("Terminal clipboard request sent; paste to verify permissions")
			s.CopyStatusUntil = time.Now().Add(5 * time.Second)
		}
	default:
	}
}
