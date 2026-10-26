package app

import (
	"github.com/Phobby/TuiSense/internal/i18n"
	"math"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/diag"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/Phobby/TuiSense/internal/theme"
	"github.com/Phobby/TuiSense/internal/ui/views"
	"github.com/thebanri/limoni/core/driver"
	"github.com/thebanri/limoni/core/terminal"
)

// HandleKeyEvent processes keyboard navigation, widget focus interactions,
// 3D camera orbit keys, and global panel shortcuts. Returns false when the user requests to quit.
func (a *Application) HandleKeyEvent(f *terminal.Frame, key driver.KeyEvent) bool {
	if key.Release {
		return true
	}
	s := a.State

	if s.ShowCopyFallback {
		if key.Type == driver.KeyEsc || key.Type == driver.KeyEnter {
			s.ShowCopyFallback = false
		}
		return true
	}

	// 0. Device selector modal captures navigation keys while open
	if s.ShowDeviceSelector {
		switch {
		case key.Type == driver.KeyEsc || (key.Type == driver.KeyRune && (key.Ch == 'c' || key.Ch == 'C' || key.Ch == 'q')):
			s.ShowDeviceSelector = false
		case key.Type == driver.KeyArrowUp && s.SelectorIndex > 0:
			s.SelectorIndex--
		case key.Type == driver.KeyArrowDown && s.SelectorIndex < len(s.Devices)-1:
			s.SelectorIndex++
		case key.Type == driver.KeyEnter || key.Type == driver.KeySpace:
			s.ShowDeviceSelector = false
			if s.SelectorIndex >= 0 && s.SelectorIndex < len(s.Devices) {
				a.SelectDevice(s.Devices[s.SelectorIndex].Key)
			}
		}
		return true
	}

	// Help captures navigation before any controller or page shortcuts.
	if s.ShowHelp {
		switch key.Type {
		case driver.KeyArrowDown:
			s.HelpScroll = min(s.HelpScrollMax, s.HelpScroll+1)
		case driver.KeyArrowUp:
			s.HelpScroll = max(0, s.HelpScroll-1)
		case driver.KeyPageDown:
			s.HelpScroll = min(s.HelpScrollMax, s.HelpScroll+8)
		case driver.KeyPageUp:
			s.HelpScroll = max(0, s.HelpScroll-8)
		case driver.KeyHome:
			s.HelpScroll = 0
		case driver.KeyEnd:
			s.HelpScroll = s.HelpScrollMax
		}
		if key.Type == driver.KeyEsc || key.Type == driver.KeyEnter || key.Type == driver.KeySpace ||
			(key.Type == driver.KeyRune && (key.Ch == '?' || key.Ch == 'q' || key.Ch == 'Q')) {
			s.ShowHelp = false
		}
		return true
	}

	if s.ActivePanel == state.PanelDiagnostics && (key.Type == driver.KeyPageUp || key.Type == driver.KeyPageDown) {
		delta := 8
		if key.Type == driver.KeyPageUp {
			delta = -8
		}
		s.DiagnosticsScroll = min(max(0, s.DiagnosticsScroll+delta), s.DiagnosticsScrollMax)
		return true
	}
	if s.ActivePanel != state.PanelDiagnostics && (key.Type == driver.KeyPageUp || key.Type == driver.KeyPageDown) {
		delta := 8
		if key.Type == driver.KeyPageUp {
			delta = -8
		}
		s.PanelScroll = min(max(0, s.PanelScroll+delta), s.PanelScrollMax)
		return true
	}
	focusedID := ""
	if f != nil && f.FocusManager != nil {
		focusedID = f.FocusManager.Focused()
	}

	// 2. Tab / Shift+Tab focus navigation
	if key.Type == driver.KeyTab {
		if f != nil && f.FocusManager != nil {
			if key.Shift {
				f.FocusManager.Prev()
			} else {
				f.FocusManager.Next()
			}
		}
		return true
	}

	if focusedID == "diag_deadzone_slider" && s.Diag != nil && s.Diag.Sticks != nil && s.DiagnosticDeadzoneSlider != nil {
		if s.DiagnosticDeadzoneSlider.HandleKey(key, 0, 25) {
			s.Diag.Sticks.PreviewDeadzone = float64(s.DiagnosticDeadzoneSlider.Value)
			return true
		}
	}
	if s.Copy.Activate(focusedID, views.CopyScope(s), key) {
		return true
	}

	// 3. If TextInput is focused on the Profiles panel, route typing to it
	if s.ActivePanel == state.PanelProfiles && focusedID == "input_profile_name" {
		if key.Type == driver.KeyEsc {
			if f != nil && f.FocusManager != nil {
				f.FocusManager.SetFocused("btn_profile_save")
			}
			return true
		}
		s.ProfileNameInput.HandleKey(key)
		return true
	}

	// 4. Esc handling: dismiss inline error -> return to Overview -> Quit
	if key.Type == driver.KeyEsc {
		if s.LastError != "" {
			s.ClearError()
			return true
		}
		if s.ActivePanel != state.PanelOverview {
			s.ActivePanel = state.PanelOverview
			return true
		}
		s.SelectedRegion = device.RegionNone
		s.RegionHint = ""
		return true
	}

	// 5. Route directional / activation keys to the currently focused widget
	if a.handleFocusedWidgetKey(focusedID, key) {
		return true
	}

	// 6a. Diagnostics shortcuts (only while the Diagnostics tab is open)
	if key.Type == driver.KeyRune && s.ActivePanel == state.PanelDiagnostics && s.Diag != nil {
		d := s.Diag
		switch key.Ch {
		case 'h', 'l':
			cats := append([]diag.Category{""}, d.Visible()...)
			idx := 0
			for i, c := range cats {
				if c == d.Active {
					idx = i
				}
			}
			step := 1
			if key.Ch == 'h' {
				step = -1
			}
			d.Select(cats[(idx+step+len(cats))%len(cats)])
			return true
		case 'o':
			views.DiagPrimaryAction(d, time.Now())
			return true
		case 'g':
			d.StartGuided()
			return true
		case 'n':
			d.NextGuided()
			return true
		case 'x':
			d.Suspend()
			return true
		}
	}

	if key.Type == driver.KeyRune && (key.Ch == 'y' || key.Ch == 'Y') {
		if s.ActivePanel == state.PanelDiagnostics {
			a.CopyText(views.BuildHealthClipboard(s, key.Ch == 'y', time.Now()))
		} else {
			a.CopyText(views.BuildClipboardReport(s, key.Ch == 'y', a.Opts.Version, time.Now()))
		}
		return true
	}

	// 6. Global Rune Shortcuts
	if key.Type == driver.KeyRune {
		switch key.Ch {
		case 'q', 'Q':
			return false
		case '?':
			s.ShowHelp = !s.ShowHelp
			return true
		case '1', '2', '3', '4', '5', '6', '7', '8', '9', '0':
			for _, e := range s.PrimaryPanels() {
				if e.Hotkey == key.Ch {
					s.ActivePanel = e.Spec.ID
					break
				}
			}
			return true
		case 'f', 'F':
			if state.PanelVisible(s.VisiblePanels(), state.PanelFirmware) {
				s.ActivePanel = state.PanelFirmware
			}
			return true
		case 'p', 'P':
			if state.PanelVisible(s.VisiblePanels(), state.PanelProfiles) {
				s.ActivePanel = state.PanelProfiles
			}
			return true
		case 'c', 'C':
			if len(s.Devices) > 0 {
				s.ShowDeviceSelector = true
				s.SelectorIndex = 0
				for i, d := range s.Devices {
					if d.Key == s.ActiveDeviceKey {
						s.SelectorIndex = i
					}
				}
			}
			return true
		case ',', '<':
			a.CycleDevice(-1)
			return true
		case '.', '>':
			a.CycleDevice(1)
			return true
		case 'e', 'E':
			a.SetMode(s.Mode.Toggle())
			return true
		case '[':
			s.CycleRegion(-1)
			return true
		case ']':
			s.CycleRegion(1)
			return true
		case 'w', 'W':
			s.AutoRotate = false
			s.RotX = math.Max(-75.0, s.RotX-6.0)
			return true
		case 's', 'S':
			s.AutoRotate = false
			s.RotX = math.Min(75.0, s.RotX+6.0)
			return true
		case 'a', 'A':
			s.AutoRotate = false
			s.RotY = math.Mod(s.RotY-8.0+360.0, 360.0)
			return true
		case 'd':
			s.AutoRotate = false
			s.RotY = math.Mod(s.RotY+8.0, 360.0)
			return true
		case 'D':
			if s.IsSimulatedDevice() {
				a.ToggleSimulatedConnection()
			}
			return true
		case 'b', 'B':
			if s.IsSimulatedDevice() {
				a.CycleSimulatedTransport()
			}
			return true
		case 'u', 'U':
			if s.IsSimulatedDevice() {
				a.ToggleSimulatedPermission()
			}
			return true
		case 'r', 'R':
			s.ResetCamera()
			return true
		case 'm', 'M':
			switch s.ShadingMode {
			case "Gouraud":
				s.ShadingMode = "Lambert"
			case "Lambert":
				s.ShadingMode = "Flat"
			case "Flat":
				s.ShadingMode = "Wireframe"
			default:
				s.ShadingMode = "Gouraud"
			}
			return true
		case 't', 'T':
			a.TestPulse()
			return true
		case '+', '=':
			s.Zoom = math.Min(1.75, s.Zoom+0.10)
			return true
		case '-':
			s.Zoom = math.Max(0.55, s.Zoom-0.10)
			return true
		}
	}

	if key.Type == driver.KeySpace {
		s.AutoRotate = !s.AutoRotate
		return true
	}

	return true
}

func (a *Application) handleFocusedWidgetKey(id string, key driver.KeyEvent) bool {
	s := a.State

	// Slider adjustments via Left/Right/Up/Down/Home/End
	adjustSlider := func(st *state.AppState, sliderID string) bool {
		switch sliderID {
		case "slider_l2_start":
			if s.L2StartSlider.HandleKey(key, 0, 255) {
				s.Config.LeftTrigger.StartPosition = uint8(s.L2StartSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: s.Config})
				return true
			}
		case "slider_l2_end":
			if s.L2EndSlider.HandleKey(key, 0, 255) {
				s.Config.LeftTrigger.EndPosition = uint8(s.L2EndSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: s.Config})
				return true
			}
		case "slider_l2_strength":
			if s.L2StrengthSlider.HandleKey(key, 0, 255) {
				s.Config.LeftTrigger.Strength = uint8(s.L2StrengthSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: s.Config})
				return true
			}
		case "slider_l2_freq":
			if s.L2FreqSlider.HandleKey(key, 0, 100) {
				s.Config.LeftTrigger.Frequency = uint8(s.L2FreqSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: s.Config})
				return true
			}
		case "slider_r2_start":
			if s.R2StartSlider.HandleKey(key, 0, 255) {
				s.Config.RightTrigger.StartPosition = uint8(s.R2StartSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: s.Config})
				return true
			}
		case "slider_r2_end":
			if s.R2EndSlider.HandleKey(key, 0, 255) {
				s.Config.RightTrigger.EndPosition = uint8(s.R2EndSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: s.Config})
				return true
			}
		case "slider_r2_strength":
			if s.R2StrengthSlider.HandleKey(key, 0, 255) {
				s.Config.RightTrigger.Strength = uint8(s.R2StrengthSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: s.Config})
				return true
			}
		case "slider_r2_freq":
			if s.R2FreqSlider.HandleKey(key, 0, 100) {
				s.Config.RightTrigger.Frequency = uint8(s.R2FreqSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetTrigger, Config: s.Config})
				return true
			}
		case "slider_hap_master":
			if s.MasterHapSlider.HandleKey(key, 0, 100) {
				s.Config.Haptics.MasterIntensity = s.MasterHapSlider.Value
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetHaptics, Config: s.Config})
				return true
			}
		case "slider_hap_left":
			if s.LeftMotorSlider.HandleKey(key, 0, 255) {
				s.Config.Haptics.LeftMotor = uint8(s.LeftMotorSlider.Value)
				if s.Config.Haptics.LinkMotors {
					s.Config.Haptics.RightMotor = uint8(s.LeftMotorSlider.Value)
					s.RightMotorSlider.Set(s.LeftMotorSlider.Value, 0, 255)
				}
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetHaptics, Config: s.Config})
				return true
			}
		case "slider_hap_right":
			if s.RightMotorSlider.HandleKey(key, 0, 255) {
				s.Config.Haptics.RightMotor = uint8(s.RightMotorSlider.Value)
				if s.Config.Haptics.LinkMotors {
					s.Config.Haptics.LeftMotor = uint8(s.RightMotorSlider.Value)
					s.LeftMotorSlider.Set(s.RightMotorSlider.Value, 0, 255)
				}
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetHaptics, Config: s.Config})
				return true
			}
		case "slider_hap_lt":
			if s.LTMotorSlider.HandleKey(key, 0, 255) {
				s.Config.Haptics.LeftTriggerMotor = uint8(s.LTMotorSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetHaptics, Config: s.Config})
				return true
			}
		case "slider_hap_rt":
			if s.RTMotorSlider.HandleKey(key, 0, 255) {
				s.Config.Haptics.RightTriggerMotor = uint8(s.RTMotorSlider.Value)
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetHaptics, Config: s.Config})
				return true
			}
		case "slider_headset_vol":
			if s.HeadsetVolSlider.HandleKey(key, 0, 100) {
				s.Config.Audio.HeadphoneVolume = s.HeadsetVolSlider.Value
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetSpeakerVol, Config: s.Config})
				return true
			}
		case "slider_speaker_vol":
			if s.SpeakerVolSlider.HandleKey(key, 0, 100) {
				s.Config.Audio.SpeakerVolume = s.SpeakerVolSlider.Value
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetSpeakerVol, Config: s.Config})
				return true
			}
		case "slider_mic_gain":
			if s.MicGainSlider.HandleKey(key, 0, 100) {
				s.Config.Audio.MicGain = s.MicGainSlider.Value
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetSpeakerVol, Config: s.Config})
				return true
			}
		case "slider_lstick_inner_dz":
			if s.LInnerDZSlider.HandleKey(key, 0, 40) {
				s.Config.Sticks.Left.InnerDeadzone = s.LInnerDZSlider.Value
				return true
			}
		case "slider_lstick_outer_dz":
			if s.LOuterDZSlider.HandleKey(key, 70, 100) {
				s.Config.Sticks.Left.OuterDeadzone = s.LOuterDZSlider.Value
				return true
			}
		case "slider_lstick_sens":
			if s.LSensSlider.HandleKey(key, 50, 150) {
				s.Config.Sticks.Left.Sensitivity = s.LSensSlider.Value
				return true
			}
		case "slider_rstick_inner_dz":
			if s.RInnerDZSlider.HandleKey(key, 0, 40) {
				s.Config.Sticks.Right.InnerDeadzone = s.RInnerDZSlider.Value
				return true
			}
		case "slider_rstick_outer_dz":
			if s.ROuterDZSlider.HandleKey(key, 70, 100) {
				s.Config.Sticks.Right.OuterDeadzone = s.ROuterDZSlider.Value
				return true
			}
		case "slider_rstick_sens":
			if s.RSensSlider.HandleKey(key, 50, 150) {
				s.Config.Sticks.Right.Sensitivity = s.RSensSlider.Value
				return true
			}
		case "slider_gyro_sens":
			if s.GyroSensSlider.HandleKey(key, 10, 200) {
				s.Config.Motion.GyroSensitivity = s.GyroSensSlider.Value
				return true
			}
		}
		return false
	}

	if adjustSlider(s, id) {
		return true
	}

	if id == "lightbar_color_picker" {
		if s.ColorPickerState.HandleKey(key, nil) {
			s.Config.Lighting.Red = s.ColorPickerState.Red
			s.Config.Lighting.Green = s.ColorPickerState.Green
			s.Config.Lighting.Blue = s.ColorPickerState.Blue
			l := s.Config.Lighting
			a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetLighting, Config: s.Config, Lighting: &l})
			return true
		}
	}

	if s.Mode == state.ModeAdvanced && (id == "profile_list" || s.ActivePanel == state.PanelProfiles) {
		profiles := s.ProfileMgr.ListFor(s.Descriptor.Family)
		if key.Type == driver.KeyArrowUp && s.ProfileListState.Selected > 0 {
			s.ProfileListState.Selected--
			return true
		}
		if key.Type == driver.KeyArrowDown && s.ProfileListState.Selected < len(profiles)-1 {
			s.ProfileListState.Selected++
			return true
		}
		if key.Type == driver.KeyEnter && id == "profile_list" {
			idx := s.ProfileListState.Selected
			if idx >= 0 && idx < len(profiles) {
				chosen := profiles[idx]
				s.Config = chosen.Config
				s.ActiveProfile = chosen.Name
				s.SyncWidgetsFromConfig()
				a.Dispatcher.Dispatch(device.Command{Type: device.CmdApplyFullConfig, Config: s.Config})
				a.savePrefs()
				if s.Toasts != nil {
					s.Toasts.Success(i18n.T("Profile Loaded"), i18n.Tf("Applied %s", chosen.Name))
				}
				return true
			}
		}
	}

	if s.ActivePanel == state.PanelLogs {
		if s.LogViewState.HandleKey(key, s.Logger.Len()) {
			return true
		}
	}

	// Enter / Space activation for focused checkboxes and buttons
	if key.Type == driver.KeyEnter || key.Type == driver.KeySpace {
		switch id {
		case "cb_link_triggers":
			s.Config.LinkTriggers = !s.Config.LinkTriggers
			return true
		case "cb_link_motors":
			s.Config.Haptics.LinkMotors = !s.Config.Haptics.LinkMotors
			return true
		case "cb_lightbar_enabled":
			s.Config.Lighting.Enabled = !s.Config.Lighting.Enabled
			l := s.Config.Lighting
			a.Dispatcher.Dispatch(device.Command{Type: device.CmdSetLighting, Config: s.Config, Lighting: &l})
			return true
		case "cb_link_sticks":
			s.Config.Sticks.LinkSticks = !s.Config.Sticks.LinkSticks
			return true
		case "cb_gyro_enabled":
			s.Config.Motion.GyroEnabled = !s.Config.Motion.GyroEnabled
			return true
		case "cb_accel_enabled":
			s.Config.Motion.AccelEnabled = !s.Config.Motion.AccelEnabled
			return true
		case "cb_sync_3d_tilt":
			s.Config.Motion.Sync3DViewTilt = !s.Config.Motion.Sync3DViewTilt
			return true
		case "cb_invert_pitch":
			s.Config.Motion.InvertPitch = !s.Config.Motion.InvertPitch
			return true
		case "cb_bind_mac":
			s.BindToMAC = !s.BindToMAC
			return true
		case "cb_settings_autorot":
			s.AutoRotate = !s.AutoRotate
			a.savePrefs()
			return true
		case "cb_settings_high_contrast":
			s.HighContrast = !s.HighContrast
			if s.HighContrast {
				s.Theme = theme.HighContrast()
			} else {
				s.Theme = theme.Default()
			}
			a.savePrefs()
			return true
		case "btn_sim_hotplug", "btn_overview_reconnect":
			a.ToggleSimulatedConnection()
			return true
		case "btn_sim_transport":
			a.CycleSimulatedTransport()
			return true
		case "btn_sim_permission":
			a.ToggleSimulatedPermission()
			return true
		}
	}

	return false
}
