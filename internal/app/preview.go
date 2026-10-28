package app

import (
	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/preview"
	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
)

// PreviewGalleryModel opens the settings of the Controller Gallery's current
// model as a preview: the same panels a connected controller of that model
// gets, on every operating system, while nothing is sent to any controller.
// A part selected in the gallery (a trigger, a paddle…) opens its settings.
func (a *Application) PreviewGalleryModel() {
	s := a.State
	var desc device.Descriptor
	found := false
	for _, id := range s.StageSpec().HardwareModelIDs {
		if d, ok := s.HardwareModels[id]; ok {
			desc, found = d, true
			break
		}
	}
	if !found {
		return
	}
	component := s.SelectedComponent
	if !s.IsPreviewDevice() {
		a.previewReturn = s.ActiveDeviceKey
	}
	if s.Diag != nil {
		s.Diag.Suspend()
	}
	if err := a.Manager.AttachPreview(preview.Key(desc.ModelID), preview.New(desc)); err != nil {
		s.SetError(i18n.T("Preview"), err.Error())
		return
	}
	s.CloseGallery()
	a.syncActiveDevice()
	s.ActivePanel = state.PanelCustomize
	s.EnsureActivePanelVisible()
	if component != "" {
		s.SelectComponent(component)
	}
}

// ExitPreview leaves a settings preview and returns to the controller that
// was active before it (or to whatever is connected).
func (a *Application) ExitPreview() {
	s := a.State
	if !s.IsPreviewDevice() {
		return
	}
	key := s.ActiveDeviceKey
	if s.Diag != nil {
		s.Diag.Suspend()
	}
	a.Manager.RemovePreview(a.previewReturn)
	delete(a.diagSessions, key)
	delete(a.simAudio, key)
	a.previewReturn = ""
	a.syncActiveDevice()
	s.ActivePanel = state.PanelOverview
	s.SelectedRegion, s.SelectedComponent, s.RegionHint = device.RegionNone, "", ""
}

// dropPreview removes a preview that is no longer selected.
func (a *Application) dropPreview() {
	for _, d := range a.State.Devices {
		if d.Source == device.SourcePreview && d.Key != a.State.ActiveDeviceKey {
			a.Manager.RemovePreview(a.State.ActiveDeviceKey)
			delete(a.diagSessions, d.Key)
			delete(a.simAudio, d.Key)
			a.previewReturn = ""
			a.syncActiveDevice()
			return
		}
	}
}
