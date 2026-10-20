package app

import (
	"testing"

	"github.com/Phobby/TuiSense/internal/i18n"
	"github.com/Phobby/TuiSense/internal/state"
	"github.com/thebanri/limoni/uitest"
)

func TestLanguageSwitchFromSettingsPersists(t *testing.T) {
	defer i18n.Set(i18n.English)
	dirs := newDirs(t)
	a := newTestApp(t, dirs, Options{ForceMock: true})
	a.State.SetMode(state.ModeEZ)
	page := uitest.Run(t, 120, 40, a.Draw)

	page.Press("7") // EZ: 1 Home … 7 Settings
	if a.State.ActivePanel != state.PanelSettings {
		t.Fatalf("expected settings panel, got %s", a.State.ActivePanel)
	}
	page.Expect(page.GetByID("settings_lang_en")).ToBeSelected()
	page.GetByID("settings_lang_tr").Click()
	if a.State.Language != i18n.Turkish || i18n.Current() != i18n.Turkish {
		t.Fatalf("clicking Türkçe should switch the UI language")
	}
	page.Expect(page.GetByRole("tab", "1 Ana Sayfa")).ToBeVisible()
	page.Expect(page.GetByRole("tab", "2 Tanılama")).ToBeVisible()
	page.Expect(page.GetByID("settings_lang_tr")).ToBeSelected()

	// The choice survives a restart (Language option left empty on purpose).
	i18n.Set(i18n.English)
	b := New(Options{ForceMock: true, ProfileDir: dirs.profiles, ConfigDir: dirs.config,
		Registry: a.Opts.Registry})
	if b.State.Language != i18n.Turkish || i18n.Current() != i18n.Turkish {
		t.Fatalf("saved language should be restored, got %s", b.State.Language)
	}

	// Switching back works the same way.
	page.GetByID("settings_lang_en").Click()
	page.Expect(page.GetByRole("tab", "1 Home")).ToBeVisible()
}

func TestAdvancedSettingsAlsoOfferLanguage(t *testing.T) {
	defer i18n.Set(i18n.English)
	a := newMockApp(t, state.ModeAdvanced)
	page := uitest.Run(t, 120, 40, a.Draw)
	a.State.ActivePanel = state.PanelSettings
	page.GetByID("settings_lang_tr").Click()
	page.Expect(page.GetByRole("tab", "Ayarlar")).ToBeVisible()
	page.Expect(page.GetByRole("tab", "Tanılama")).ToBeVisible()
}
