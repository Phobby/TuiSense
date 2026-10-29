package views

import (
	"fmt"
	"github.com/Phobby/TuiSense/internal/i18n"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/core/terminal"
	"github.com/thebanri/limoni/widgets"
)

// renderProfilesPanel renders the named profile manager with Save, Load, Delete,
// Factory Reset, and per-controller MAC address binding. Only universal profiles
// and those saved for the active device family are listed.
func renderProfilesPanel(f *terminal.Frame, area cell.Rect, pc PanelContext) {
	s := pc.S
	dispatch := pc.Dispatch
	p := s.Theme

	inner := panelFrame(f, area, s, i18n.T("Profiles"))
	y := inner.Y

	profiles := s.ProfileMgr.ListFor(s.Descriptor.Family)
	items := make([]string, len(profiles))
	for i, prof := range profiles {
		tag := i18n.T("[User]")
		if prof.BuiltIn {
			tag = i18n.T("[Preset]")
		} else if prof.Family != "" {
			tag = "[" + prof.Family + "]"
		}
		activeMark := "  "
		if prof.Name == s.ActiveProfile {
			activeMark = "● "
		}
		macSuffix := ""
		if prof.ControllerMAC != "" {
			macSuffix = i18n.Tf(" (MAC: %s)", prof.ControllerMAC)
		}
		items[i] = fmt.Sprintf("%s%-20s %-8s — %s%s", activeMark, prof.Name, tag, i18n.T(prof.Description), macSuffix)
	}

	if s.ProfileListState.Selected < 0 || s.ProfileListState.Selected >= len(profiles) {
		s.ProfileListState.Selected = 0
	}

	f.Buffer.SetStringWithin(inner.X, y, i18n.Tf("Active: %s  ·  %s", s.ActiveProfile, s.ProfileMgr.Dir()), p.SectionHeaderStyle(), inner.Width)
	y++

	listH := uint16(min(len(items)+1, max(3, int(inner.Height)-8)))
	listWidget := widgets.NewList(items...).
		WithID("profile_list").
		WithState(s.ProfileListState).
		WithStyle(p.PanelStyle()).
		WithSelectedStyle(cell.Style{Fg: p.BgCanvas, Bg: p.AccentCyan, Modifier: cell.ModifierBold})
	f.RenderWidget(listWidget, cell.NewRect(inner.X, y, inner.Width, listH))
	y += listH + 2 // a blank row, then the buttons' top edge

	// Action Buttons: Load, Reset Defaults, Delete
	if y < inner.Y+inner.Height {
		pillRow(f, inner.X, y, inner.X+inner.Width, s, "", 0, []choice{
			{ID: "btn_profile_load", Label: i18n.T("✓ Load Selected"), OnSelect: func() {
				idx := s.ProfileListState.Selected
				if idx >= 0 && idx < len(profiles) {
					chosen := profiles[idx]
					s.Config = chosen.Config
					s.ActiveProfile = chosen.Name
					s.SyncWidgetsFromConfig()
					if dispatch != nil {
						dispatch(device.Command{Type: device.CmdApplyFullConfig, Config: s.Config})
					}
					pc.prefsChanged()
					if s.Toasts != nil {
						s.Toasts.Success(i18n.T("Profile Loaded"), i18n.Tf("Applied profile %q", chosen.Name))
					}
				}
			}},
			{ID: "btn_profile_reset", Label: i18n.T("↺ Factory Reset"), OnSelect: func() {
				s.Config = s.Descriptor.NewDefaultConfig()
				s.ActiveProfile = "Factory Defaults"
				s.SyncWidgetsFromConfig()
				if dispatch != nil {
					dispatch(device.Command{Type: device.CmdApplyFullConfig, Config: s.Config})
				}
				if s.Toasts != nil {
					s.Toasts.Info(i18n.T("Factory Reset"), i18n.Tf("Restored default %s configuration", i18n.T(s.Descriptor.ShortName)))
				}
			}},
			{ID: "btn_profile_delete", Label: i18n.T("✕ Delete"), OnSelect: func() {
				idx := s.ProfileListState.Selected
				if idx >= 0 && idx < len(profiles) {
					target := profiles[idx]
					if err := s.ProfileMgr.Delete(target.Name); err != nil {
						s.SetError(i18n.T("Profile Delete"), err.Error())
					} else if s.Toasts != nil {
						s.Toasts.Warning(i18n.T("Profile Deleted"), i18n.Tf("Removed profile %q", target.Name))
					}
				}
			}},
		})
		y += pillRowH
	}

	// Save New / Overwrite Profile Row
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, i18n.T("Save Name:"), p.LabelStyle(), 11)
		inputW := uint16(24)
		if inner.Width < 46 {
			inputW = max(10, inner.Width-24)
		}
		ti := widgets.NewTextInput("input_profile_name").
			WithState(s.ProfileNameInput).
			WithPlaceholder(i18n.T("Enter profile name...")).
			WithStyle(cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated}).
			WithFocusedStyle(cell.Style{Fg: p.AccentCyan, Bg: p.BgElevated, Modifier: cell.ModifierBold})
		f.RenderWidget(ti, cell.NewRect(inner.X+11, y, inputW, 1))

		pillRow(f, inner.X+11+inputW+1, y, inner.X+inner.Width, s, "", 0, []choice{
			{ID: "btn_profile_save", Label: i18n.T("Save Profile"), OnSelect: func() {
				name := s.ProfileNameInput.Value()
				mac := ""
				if s.BindToMAC {
					mac = s.DeviceInfo.MACAddress
				}
				prof, err := s.ProfileMgr.Save(name, i18n.Tf("Custom %s profile", s.Descriptor.ShortName), mac, s.Descriptor.Family, s.Config)
				if err != nil {
					s.SetError(i18n.T("Profile Save"), err.Error())
					return
				}
				s.ActiveProfile = prof.Name
				pc.prefsChanged()
				if s.Logger != nil {
					s.Logger.Info("PROFILES", "Saved profile %q (MAC=%s)", prof.Name, mac)
				}
				if s.Toasts != nil {
					s.Toasts.Success(i18n.T("Profile Saved"), i18n.Tf("Saved %q to disk", prof.Name))
				}
			}},
		})
		y += 2
	}

	if y < inner.Y+inner.Height {
		macLabel := i18n.Tf("Auto-bind saved profile to controller MAC (%s)", nonEmptyStr(s.DeviceInfo.MACAddress, "N/A"))
		f.RenderWidget(widgets.Checkbox{
			ID:           "cb_bind_mac",
			Checked:      &s.BindToMAC,
			Label:        macLabel,
			Style:        p.MutedStyle(),
			FocusedStyle: p.ButtonFocusedStyle(),
		}, cell.NewRect(inner.X, y, inner.Width, 1))
	}
}
