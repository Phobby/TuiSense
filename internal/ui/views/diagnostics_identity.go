package views

import (
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/core/cell"
)

func identityLabel(st string) string {
	switch st {
	case "identified":
		return i18n.T("Identified")
	case "confirmed":
		return i18n.T("Confirmed by you")
	case "probable":
		return i18n.T("Probable")
	}
	return i18n.T("Not identified")
}

// renderIdentity shows how the model was recognized and, when it was only
// inferred, lets the user confirm or reject it. Nothing is sent to the device.
func renderIdentity(c *cursor, s *state.AppState, cb Callbacks) {
	info := s.DeviceInfo
	if info.IdentityState == "" || !c.ok() {
		return
	}
	p := c.p()
	col := p.Success
	switch info.IdentityState {
	case "probable":
		col = p.Warning
	case "unidentified":
		col = p.FgMuted
	}
	c.gap()
	c.line(i18n.T("Identification: ")+identityLabel(info.IdentityState), cell.Style{Fg: col, Bg: p.BgPanel, Modifier: cell.ModifierBold})
	if info.Identification != "" {
		c.wrap(i18n.T(info.Identification), p.MutedStyle(), 3)
	}
	if cb.OnConfirmIdentity == nil || info.IdentityKey == "" {
		return
	}
	key := info.IdentityKey
	switch {
	case info.IdentityState == "confirmed":
		c.pills("", 0, []choice{{ID: "identity_forget", Label: i18n.T("Forget my confirmation"), OnSelect: func() { cb.OnConfirmIdentity(key, "") }}})
	case info.IdentityCandidateID != "" && (info.IdentityState == "probable" || info.IdentityState == "unidentified"):
		cand := info.IdentityCandidateID
		c.wrap(i18n.Tf("Is this your %s?", i18n.T(info.IdentityCandidateName)), p.LabelStyle(), 2)
		items := []choice{{ID: "identity_yes", Label: i18n.T("✓ Yes, it is"), OnSelect: func() { cb.OnConfirmIdentity(key, cand) }}}
		if info.IdentityState == "probable" {
			items = append(items, choice{ID: "identity_no", Label: i18n.T("✕ No, it is another controller"), OnSelect: func() { cb.OnConfirmIdentity(key, device.GenericGamepadModelID) }})
		}
		c.pills("", 0, items)
		c.wrap(i18n.Tf("Your answer is remembered for USB ID %s only (no serial number is stored).", key), p.MutedStyle(), 2)
	}
}

// renderModeGuide lists the controller's documented input modes.
func renderModeGuide(c *cursor, desc device.Descriptor) {
	if len(desc.ModeGuide) == 0 || c.remaining() < 3 {
		return
	}
	p := c.p()
	c.gap()
	c.line(i18n.T("Input modes (from the manufacturer's manual)"), p.SectionHeaderStyle())
	for _, m := range desc.ModeGuide {
		c.wrap("· "+i18n.T(m), p.MutedStyle(), 2)
	}
	c.wrap(i18n.T("Switching modes reconnects the controller; TuiSense re-detects it automatically. TuiSense never switches modes itself."), p.MutedStyle(), 2)
}
