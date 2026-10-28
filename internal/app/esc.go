package app

import (
	"time"

	"github.com/thebanri/limoni/core/driver"
)

// escRepair undoes a terminal-input quirk seen while Esc is held down.
//
// A held Esc streams ESC bytes, and moving the mouse interleaves SGR mouse
// reports ("ESC [ < 35 ; 40 ; 10 M"). Limoni decodes "ESC ESC" as a single
// Alt+Esc key, so when the pairing lands on a mouse report's own ESC, the rest
// of the report ("[<35;40;10M") arrives as ordinary key presses: '[' cycles
// controller parts, '3' switches tabs, 'M' changes the 3D shading. escRepair
// turns Alt+Esc back into Esc and drops the orphaned control sequence.
type escRepair int

const (
	escIdle     escRepair = iota
	escAfterAlt           // Alt+Esc seen; a '[' here starts an orphaned CSI
	escInCSI              // inside the orphaned CSI, until its final byte
)

// orphanWindow bounds how long after Alt+Esc the rest of a sequence may
// arrive; the bytes come from the same terminal read, so far less is needed.
const orphanWindow = 100 * time.Millisecond

// repairKey returns the key to handle, or false to drop it.
func (a *Application) repairKey(key driver.KeyEvent, now time.Time) (driver.KeyEvent, bool) {
	if a.escFix != escIdle && now.Sub(a.escFixAt) > orphanWindow {
		a.escFix = escIdle
	}
	plain := key.Type == driver.KeyRune && !key.Alt && !key.Ctrl
	switch a.escFix {
	case escAfterAlt:
		a.escFix = escIdle
		if plain && key.Ch == '[' {
			a.escFix = escInCSI
			return key, false
		}
	case escInCSI:
		switch {
		case plain && key.Ch >= 0x20 && key.Ch <= 0x3f: // parameters and intermediates
			return key, false
		case plain && key.Ch >= 0x40 && key.Ch <= 0x7e: // final byte
			a.escFix = escIdle
			return key, false
		}
		a.escFix = escIdle
	}
	if key.Type == driver.KeyRune && key.Alt && key.Ch == 0x1b {
		a.escFix, a.escFixAt = escAfterAlt, now
		return driver.KeyEvent{Type: driver.KeyEsc, Repeat: key.Repeat}, true
	}
	return key, true
}

// escHoldGap separates two Esc presses from one held-down Esc: keyboard
// auto-repeat starts after roughly 250–660 ms and then repeats every 30–50 ms.
const escHoldGap = 700 * time.Millisecond

// escHeld records an Esc press and reports whether it continues a held Esc
// (an auto-repeat) rather than being a new press.
func (a *Application) escHeld(key driver.KeyEvent, now time.Time) bool {
	held := key.Repeat || (!a.escAt.IsZero() && now.Sub(a.escAt) < escHoldGap)
	a.escAt = now
	return held
}

// handleQuitConfirm owns the keyboard while the quit dialog is open: Enter or
// Y/E (Evet) quits, Esc or N/H (Hayır) stays. It returns false to quit.
func (a *Application) handleQuitConfirm(key driver.KeyEvent) bool {
	switch {
	case key.Type == driver.KeyEnter, key.Type == driver.KeyRune && (key.Ch == 'y' || key.Ch == 'Y' || key.Ch == 'e' || key.Ch == 'E'):
		return false
	case key.Type == driver.KeyEsc, key.Type == driver.KeyRune && (key.Ch == 'n' || key.Ch == 'N' || key.Ch == 'h' || key.Ch == 'H'):
		a.State.ShowQuitConfirm = false
	}
	return true
}
