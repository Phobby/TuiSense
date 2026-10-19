package views

import (
	"fmt"

	"github.com/Phobby/TuiSense/internal/device"
	uiwidgets "github.com/Phobby/TuiSense/internal/ui/widgets"
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

	inner := panelFrame(f, area, s, "Profiles")
	y := inner.Y

	profiles := s.ProfileMgr.ListFor(s.Descriptor.Family)
	items := make([]string, len(profiles))
	for i, prof := range profiles {
		tag := "[User]"
		if prof.BuiltIn {
			tag = "[Preset]"
		} else if prof.Family != "" {
			tag = "[" + prof.Family + "]"
		}
		activeMark := "  "
		if prof.Name == s.ActiveProfile {
			activeMark = "● "
		}
		macSuffix := ""
		if prof.ControllerMAC != "" {
			macSuffix = fmt.Sprintf(" (MAC: %s)", prof.ControllerMAC)
		}
		items[i] = fmt.Sprintf("%s%-20s %-8s — %s%s", activeMark, prof.Name, tag, prof.Description, macSuffix)
	}

	if s.ProfileListState.Selected < 0 || s.ProfileListState.Selected >= len(profiles) {
		s.ProfileListState.Selected = 0
	}

	f.Buffer.SetStringWithin(inner.X, y, fmt.Sprintf("Active: %s  ·  %s", s.ActiveProfile, s.ProfileMgr.Dir()), p.SectionHeaderStyle(), inner.Width)
	y++

	listH := uint16(min(len(items)+1, max(3, int(inner.Height)-8)))
	listWidget := widgets.NewList(items...).
		WithID("profile_list").
		WithState(s.ProfileListState).
		WithStyle(p.PanelStyle()).
		WithSelectedStyle(cell.Style{Fg: p.BgCanvas, Bg: p.AccentCyan, Modifier: cell.ModifierBold})
	f.RenderWidget(listWidget, cell.NewRect(inner.X, y, inner.Width, listH))
	y += listH + 1

	// Action Buttons: Load, Reset Defaults, Delete
	if y < inner.Y+inner.Height {
		btnLoad := widgets.Button{
			ID:           "btn_profile_load",
			Label:        "✓ Load Selected",
			Style:        p.ButtonStyle(true),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress: func() {
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
						s.Toasts.Success("Profile Loaded", fmt.Sprintf("Applied profile %q", chosen.Name))
					}
				}
			},
		}

		btnReset := widgets.Button{
			ID:           "btn_profile_reset",
			Label:        "↺ Factory Reset",
			Style:        p.ButtonStyle(false),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress: func() {
				s.Config = s.Descriptor.NewDefaultConfig()
				s.ActiveProfile = "Factory Defaults"
				s.SyncWidgetsFromConfig()
				if dispatch != nil {
					dispatch(device.Command{Type: device.CmdApplyFullConfig, Config: s.Config})
				}
				if s.Toasts != nil {
					s.Toasts.Info("Factory Reset", "Restored default "+s.Descriptor.ShortName+" configuration")
				}
			},
		}

		btnDelete := widgets.Button{
			ID:           "btn_profile_delete",
			Label:        "✕ Delete",
			Style:        p.ButtonStyle(false),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress: func() {
				idx := s.ProfileListState.Selected
				if idx >= 0 && idx < len(profiles) {
					target := profiles[idx]
					if err := s.ProfileMgr.Delete(target.Name); err != nil {
						s.SetError("Profile Delete", err.Error())
					} else if s.Toasts != nil {
						s.Toasts.Warning("Profile Deleted", fmt.Sprintf("Removed profile %q", target.Name))
					}
				}
			},
		}

		w1, _ := btnLoad.SizeHint(inner)
		w2, _ := btnReset.SizeHint(inner)
		w3, _ := btnDelete.SizeHint(inner)
		uiwidgets.RenderButton(f, cell.NewRect(inner.X, y, w1, 1), btnLoad)
		if inner.X+w1+1+w2 <= inner.X+inner.Width {
			uiwidgets.RenderButton(f, cell.NewRect(inner.X+w1+1, y, w2, 1), btnReset)
		}
		if inner.X+w1+w2+2+w3 <= inner.X+inner.Width {
			uiwidgets.RenderButton(f, cell.NewRect(inner.X+w1+w2+2, y, w3, 1), btnDelete)
		}
		y += 2
	}

	// Save New / Overwrite Profile Row
	if y < inner.Y+inner.Height {
		f.Buffer.SetStringWithin(inner.X, y, "Save Name:", p.LabelStyle(), 11)
		inputW := uint16(24)
		if inner.Width < 46 {
			inputW = max(10, inner.Width-24)
		}
		ti := widgets.NewTextInput("input_profile_name").
			WithState(s.ProfileNameInput).
			WithPlaceholder("Enter profile name...").
			WithStyle(cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated}).
			WithFocusedStyle(cell.Style{Fg: p.AccentCyan, Bg: p.BgElevated, Modifier: cell.ModifierBold})
		f.RenderWidget(ti, cell.NewRect(inner.X+11, y, inputW, 1))

		btnSave := widgets.Button{
			ID:           "btn_profile_save",
			Label:        "💾 Save Profile",
			Style:        p.ButtonStyle(true),
			FocusedStyle: p.ButtonFocusedStyle(),
			OnPress: func() {
				name := s.ProfileNameInput.Value()
				mac := ""
				if s.BindToMAC {
					mac = s.DeviceInfo.MACAddress
				}
				prof, err := s.ProfileMgr.Save(name, "Custom "+s.Descriptor.ShortName+" profile", mac, s.Descriptor.Family, s.Config)
				if err != nil {
					s.SetError("Profile Save", err.Error())
					return
				}
				s.ActiveProfile = prof.Name
				pc.prefsChanged()
				if s.Logger != nil {
					s.Logger.Info("PROFILES", "Saved profile %q (MAC=%s)", prof.Name, mac)
				}
				if s.Toasts != nil {
					s.Toasts.Success("Profile Saved", fmt.Sprintf("Saved %q to disk", prof.Name))
				}
			},
		}
		wSave, _ := btnSave.SizeHint(inner)
		if inner.X+11+inputW+1+wSave <= inner.X+inner.Width {
			uiwidgets.RenderButton(f, cell.NewRect(inner.X+11+inputW+1, y, wSave, 1), btnSave)
		}
		y++
	}

	if y < inner.Y+inner.Height {
		macLabel := fmt.Sprintf("Auto-bind saved profile to controller MAC (%s)", nonEmptyStr(s.DeviceInfo.MACAddress, "N/A"))
		f.RenderWidget(widgets.Checkbox{
			ID:           "cb_bind_mac",
			Checked:      &s.BindToMAC,
			Label:        macLabel,
			Style:        p.MutedStyle(),
			FocusedStyle: p.ButtonFocusedStyle(),
		}, cell.NewRect(inner.X, y, inner.Width, 1))
	}
}
