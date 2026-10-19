package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ygzztrk/tuisense/internal/device"
	"github.com/ygzztrk/tuisense/internal/system"
)

// Profile represents a named controller configuration preset stored on disk or built-in.
type Profile struct {
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	ControllerMAC string        `json:"controller_mac,omitempty"`
	Family        string        `json:"family,omitempty"` // "" = universal (any controller)
	BuiltIn       bool          `json:"built_in"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Config        device.Config `json:"config"`
}

// ProfileManager manages saving, loading, deleting, and per-controller binding of profiles.
type ProfileManager struct {
	mu       sync.RWMutex
	dir      string
	profiles map[string]Profile
	bindings map[string]string // Controller MAC/ID -> Profile Name
}

// NewProfileManager initializes a ProfileManager rooted at dir (defaults to $XDG_CONFIG_HOME/tuisense/profiles).
func NewProfileManager(dir string) *ProfileManager {
	if dir == "" {
		dir = system.ProfilesDir()
	}
	pm := &ProfileManager{
		dir:      dir,
		profiles: make(map[string]Profile),
		bindings: make(map[string]string),
	}
	pm.seedBuiltins()
	_ = pm.LoadFromDisk()
	return pm
}

// Dir returns the filesystem directory where user profiles are stored.
func (pm *ProfileManager) Dir() string {
	return pm.dir
}

func (pm *ProfileManager) seedBuiltins() {
	now := time.Now()

	// 1. Astro Standard
	std := device.DefaultConfig()
	std.LeftTrigger = device.BuiltinTriggerPresets[1].Config
	std.RightTrigger = device.BuiltinTriggerPresets[2].Config
	pm.profiles["Astro Standard"] = Profile{
		Name:        "Astro Standard",
		Description: "Balanced PlayStation blue lighting with bow L2 & crisp weapon R2.",
		BuiltIn:     true,
		UpdatedAt:   now,
		Config:      std,
	}

	// 2. Competitive FPS
	fps := device.DefaultConfig()
	fps.Lighting.Red, fps.Lighting.Green, fps.Lighting.Blue = 255, 45, 85
	fps.Lighting.Brightness = device.BrightnessMedium
	fps.LeftTrigger = device.TriggerConfig{
		Mode:          device.TriggerModeWeapon,
		StartPosition: 45,
		EndPosition:   115,
		Strength:      210,
	}
	fps.RightTrigger = device.TriggerConfig{
		Mode:          device.TriggerModeWeapon,
		StartPosition: 35,
		EndPosition:   100,
		Strength:      240,
	}
	fps.Haptics.MasterIntensity = 30
	fps.Sticks.Left = device.StickAxisConfig{InnerDeadzone: 3, OuterDeadzone: 98, Sensitivity: 115, Curve: device.CurveAggressive}
	fps.Sticks.Right = device.StickAxisConfig{InnerDeadzone: 2, OuterDeadzone: 99, Sensitivity: 105, Curve: device.CurvePrecision}
	pm.profiles["Competitive FPS"] = Profile{
		Name:        "Competitive FPS",
		Description: "Hair-trigger L2/R2 snap breaks, ultra-low deadzones, reduced vibration.",
		BuiltIn:     true,
		UpdatedAt:   now,
		Config:      fps,
	}

	// 3. Sim Racing ABS
	race := device.DefaultConfig()
	race.Lighting.Red, race.Lighting.Green, race.Lighting.Blue = 0, 229, 255
	race.LeftTrigger = device.BuiltinTriggerPresets[3].Config // ABS Brake Pedal
	race.RightTrigger = device.TriggerConfig{
		Mode:          device.TriggerModeResistance,
		StartPosition: 20,
		EndPosition:   255,
		Strength:      175,
	}
	race.Haptics.MasterIntensity = 100
	race.Sticks.Left = device.StickAxisConfig{InnerDeadzone: 4, OuterDeadzone: 98, Sensitivity: 100, Curve: device.CurveDynamic}
	pm.profiles["Sim Racing ABS"] = Profile{
		Name:        "Sim Racing ABS",
		Description: "ABS brake shudder on L2, progressive throttle gas pedal on R2.",
		BuiltIn:     true,
		UpdatedAt:   now,
		Config:      race,
	}

	// 4. Stealth Night Mode
	stealth := device.DefaultConfig()
	stealth.Lighting.Red, stealth.Lighting.Green, stealth.Lighting.Blue = 90, 40, 180
	stealth.Lighting.Brightness = device.BrightnessLow
	stealth.Lighting.PlayerMask = 0x00
	stealth.Lighting.MicLED = device.MicLEDOff
	stealth.LeftTrigger = device.DefaultTriggerConfig()
	stealth.RightTrigger = device.DefaultTriggerConfig()
	stealth.Haptics.MasterIntensity = 40
	stealth.Audio.SpeakerVolume = 25
	pm.profiles["Stealth Night Mode"] = Profile{
		Name:        "Stealth Night Mode",
		Description: "Dimmed cosmic violet LEDs, player indicators off, quiet haptics.",
		BuiltIn:     true,
		UpdatedAt:   now,
		Config:      stealth,
	}
}

// List returns all profiles sorted with built-ins first, then alphabetically.
func (pm *ProfileManager) List() []Profile {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	out := make([]Profile, 0, len(pm.profiles))
	for _, p := range pm.profiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].BuiltIn != out[j].BuiltIn {
			return out[i].BuiltIn
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// ListFor returns universal profiles plus those saved for the given device family.
func (pm *ProfileManager) ListFor(family string) []Profile {
	all := pm.List()
	out := all[:0]
	for _, p := range all {
		if p.Family == "" || p.Family == family {
			out = append(out, p)
		}
	}
	return out
}

// Get retrieves a profile by name.
func (pm *ProfileManager) Get(name string) (Profile, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.profiles[name]
	return p, ok
}

// Save persists a named profile to disk and optionally binds it to a controller MAC.
func (pm *ProfileManager) Save(name, description, controllerMAC, family string, cfg device.Config) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Profile{}, fmt.Errorf("profile name cannot be empty")
	}
	if description == "" {
		description = "Custom user configuration profile."
	}

	pm.mu.Lock()
	existing, exists := pm.profiles[name]
	p := Profile{
		Name:          name,
		Description:   description,
		ControllerMAC: controllerMAC,
		Family:        family,
		BuiltIn:       exists && existing.BuiltIn,
		UpdatedAt:     time.Now(),
		Config:        cfg,
	}
	pm.profiles[name] = p
	if controllerMAC != "" {
		pm.bindings[controllerMAC] = name
	}
	pm.mu.Unlock()

	if err := os.MkdirAll(pm.dir, 0o755); err != nil {
		return p, fmt.Errorf("create profile dir: %w", err)
	}
	filePath := filepath.Join(pm.dir, sanitizeFilename(name)+".json")
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return p, err
	}
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		return p, fmt.Errorf("write profile %s: %w", filePath, err)
	}
	_ = pm.saveBindings()
	return p, nil
}

// Delete removes a user profile. Built-in profiles are reset to factory defaults if deleted.
func (pm *ProfileManager) Delete(name string) error {
	pm.mu.Lock()
	p, ok := pm.profiles[name]
	if !ok {
		pm.mu.Unlock()
		return fmt.Errorf("profile %q not found", name)
	}
	if p.BuiltIn {
		pm.mu.Unlock()
		return fmt.Errorf("built-in profile %q cannot be deleted", name)
	}
	delete(pm.profiles, name)
	for mac, bound := range pm.bindings {
		if bound == name {
			delete(pm.bindings, mac)
		}
	}
	pm.mu.Unlock()

	filePath := filepath.Join(pm.dir, sanitizeFilename(name)+".json")
	_ = os.Remove(filePath)
	_ = pm.saveBindings()
	return nil
}

// BindController associates a controller MAC/ID with a preferred profile name.
func (pm *ProfileManager) BindController(controllerKey, profileName string) error {
	if controllerKey == "" {
		return nil
	}
	pm.mu.Lock()
	pm.bindings[controllerKey] = profileName
	pm.mu.Unlock()
	return pm.saveBindings()
}

// BoundProfile returns the profile bound to controllerKey, if any.
func (pm *ProfileManager) BoundProfile(controllerKey string) (Profile, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	name, ok := pm.bindings[controllerKey]
	if !ok {
		return Profile{}, false
	}
	p, exists := pm.profiles[name]
	return p, exists
}

// LoadFromDisk reads all *.json profiles from pm.dir.
func (pm *ProfileManager) LoadFromDisk() error {
	entries, err := os.ReadDir(pm.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if e.Name() == "_bindings.json" {
			if data, err := os.ReadFile(filepath.Join(pm.dir, e.Name())); err == nil {
				_ = json.Unmarshal(data, &pm.bindings)
			}
			continue
		}
		data, err := os.ReadFile(filepath.Join(pm.dir, e.Name()))
		if err != nil {
			continue
		}
		var p Profile
		if err := json.Unmarshal(data, &p); err == nil && p.Name != "" {
			pm.profiles[p.Name] = p
		}
	}
	return nil
}

func (pm *ProfileManager) saveBindings() error {
	pm.mu.RLock()
	data, err := json.MarshalIndent(pm.bindings, "", "  ")
	pm.mu.RUnlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(pm.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pm.dir, "_bindings.json"), data, 0o644)
}

func sanitizeFilename(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_':
			sb.WriteRune(r)
		case r == ' ':
			sb.WriteByte('_')
		}
	}
	out := sb.String()
	if out == "" {
		return "profile"
	}
	return out
}
