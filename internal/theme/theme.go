package theme

import (
	"github.com/thebanri/limoni/core/cell"
	"github.com/thebanri/limoni/widgets"
)

// Palette defines the PlayStation-inspired dark telemetry color scheme.
type Palette struct {
	Name string

	// Backgrounds
	BgCanvas   cell.Color // Deep space obsidian (#0B0E17)
	BgPanel    cell.Color // Elevated panel surface (#111625)
	BgElevated cell.Color // Card / active control surface (#192036)
	BgSubtle   cell.Color // Muted track / badge background (#232C47)

	// Foregrounds
	FgPrimary   cell.Color // Crisp silver-white (#EEF2FF)
	FgSecondary cell.Color // Cool slate (#A5B4FC)
	FgMuted     cell.Color // Dimmed label/hint (#64748B)

	// Accents (Astro Blue & DualSense Cosmic Violet)
	AccentBlue   cell.Color // Electric PlayStation-inspired blue (#2563EB / #38BDF8)
	AccentCyan   cell.Color // Telemetry neon cyan (#00E5FF)
	AccentPurple cell.Color // Cosmic violet (#8B5CF6)
	AccentPink   cell.Color // Pulse magenta (#EC4899)

	// Semantic Status
	Success cell.Color // Connected / Charged (#10B981)
	Warning cell.Color // Charging / Caution (#F59E0B)
	Danger  cell.Color // Disconnected / Permission Error (#EF4444)

	// Controller 3D Shell Colors
	ShellWhite     cell.Color // Classic two-tone white shell (#E2E8F0)
	ShellGripDark  cell.Color // Dark inner grip & buttons (#1E293B)
	ShellTouchpad  cell.Color // Touchpad matte surface (#CBD5E1)
	ShellHighlight cell.Color // Selected region highlight (#00E5FF)
}

// Default returns the default DualSense dark theme palette.
func Default() Palette {
	return Palette{
		Name:           "Cosmic Dark",
		BgCanvas:       cell.NewColorRGB(10, 13, 22),
		BgPanel:        cell.NewColorRGB(16, 21, 36),
		BgElevated:     cell.NewColorRGB(24, 32, 54),
		BgSubtle:       cell.NewColorRGB(36, 47, 76),
		FgPrimary:      cell.NewColorRGB(238, 242, 255),
		FgSecondary:    cell.NewColorRGB(165, 180, 252),
		FgMuted:        cell.NewColorRGB(108, 124, 156),
		AccentBlue:     cell.NewColorRGB(56, 139, 255),
		AccentCyan:     cell.NewColorRGB(0, 229, 255),
		AccentPurple:   cell.NewColorRGB(147, 83, 255),
		AccentPink:     cell.NewColorRGB(236, 72, 153),
		Success:        cell.NewColorRGB(16, 215, 136),
		Warning:        cell.NewColorRGB(245, 176, 30),
		Danger:         cell.NewColorRGB(244, 67, 80),
		ShellWhite:     cell.NewColorRGB(215, 224, 240),
		ShellGripDark:  cell.NewColorRGB(42, 52, 74),
		ShellTouchpad:  cell.NewColorRGB(185, 198, 222),
		ShellHighlight: cell.NewColorRGB(0, 229, 255),
	}
}

// HighContrast returns a high-contrast theme palette for accessibility.
func HighContrast() Palette {
	return Palette{
		Name:           "High Contrast",
		BgCanvas:       cell.NewColorRGB(0, 0, 0),
		BgPanel:        cell.NewColorRGB(8, 10, 16),
		BgElevated:     cell.NewColorRGB(20, 24, 38),
		BgSubtle:       cell.NewColorRGB(40, 48, 72),
		FgPrimary:      cell.NewColorRGB(255, 255, 255),
		FgSecondary:    cell.NewColorRGB(200, 225, 255),
		FgMuted:        cell.NewColorRGB(150, 165, 190),
		AccentBlue:     cell.NewColorRGB(80, 170, 255),
		AccentCyan:     cell.NewColorRGB(0, 255, 255),
		AccentPurple:   cell.NewColorRGB(190, 120, 255),
		AccentPink:     cell.NewColorRGB(255, 100, 180),
		Success:        cell.NewColorRGB(0, 255, 128),
		Warning:        cell.NewColorRGB(255, 215, 0),
		Danger:         cell.NewColorRGB(255, 70, 70),
		ShellWhite:     cell.NewColorRGB(240, 245, 255),
		ShellGripDark:  cell.NewColorRGB(60, 75, 105),
		ShellTouchpad:  cell.NewColorRGB(210, 220, 240),
		ShellHighlight: cell.NewColorRGB(0, 255, 255),
	}
}

// LimoniTheme converts the Palette into a widgets.Theme for Limoni's cascading theme system.
func (p Palette) LimoniTheme() widgets.Theme {
	return widgets.Theme{
		Colors: widgets.ThemeColors{
			Primary:    p.AccentCyan,
			Secondary:  p.AccentPurple,
			Background: p.BgCanvas,
			Surface:    p.BgPanel,
			Border:     p.BgSubtle,
			Text:       p.FgPrimary,
			Muted:      p.FgMuted,
			Success:    p.Success,
			Warning:    p.Warning,
			Error:      p.Danger,
		},
		Base:   cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel},
		Border: cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel},
		Focus:  cell.Style{Fg: p.AccentCyan, Bg: p.BgElevated, Modifier: cell.ModifierBold},
	}
}

// Style Helpers
func (p Palette) CanvasStyle() cell.Style {
	return cell.Style{Fg: p.FgPrimary, Bg: p.BgCanvas}
}

func (p Palette) PanelStyle() cell.Style {
	return cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel}
}

func (p Palette) PanelBorder(focused bool) cell.Style {
	if focused {
		return cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold}
	}
	return cell.Style{Fg: p.AccentPurple, Bg: p.BgPanel}
}

// CardBorder is the calm default border for cards and inspector panels.
func (p Palette) CardBorder() cell.Style {
	return cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}
}

// CardTitle styles card titles: readable, restrained, never shouting.
func (p Palette) CardTitle() cell.Style {
	return cell.Style{Fg: p.FgSecondary, Bg: p.BgPanel, Modifier: cell.ModifierBold}
}

func (p Palette) SubtleBorder() cell.Style {
	return cell.Style{Fg: p.BgSubtle, Bg: p.BgPanel}
}

func (p Palette) TitleStyle() cell.Style {
	return cell.Style{Fg: p.AccentCyan, Bg: p.BgPanel, Modifier: cell.ModifierBold}
}

func (p Palette) SectionHeaderStyle() cell.Style {
	return cell.Style{Fg: p.AccentPurple, Bg: p.BgPanel, Modifier: cell.ModifierBold}
}

func (p Palette) LabelStyle() cell.Style {
	return cell.Style{Fg: p.FgSecondary, Bg: p.BgPanel}
}

func (p Palette) ValueStyle() cell.Style {
	return cell.Style{Fg: p.FgPrimary, Bg: p.BgPanel, Modifier: cell.ModifierBold}
}

func (p Palette) MutedStyle() cell.Style {
	return cell.Style{Fg: p.FgMuted, Bg: p.BgPanel}
}

func (p Palette) BadgeStyle(fg, bg cell.Color) cell.Style {
	return cell.Style{Fg: fg, Bg: bg, Modifier: cell.ModifierBold}
}

func (p Palette) ButtonStyle(active bool) cell.Style {
	if active {
		return cell.Style{Fg: p.BgCanvas, Bg: p.AccentCyan, Modifier: cell.ModifierBold}
	}
	return cell.Style{Fg: p.FgPrimary, Bg: p.BgElevated}
}

func (p Palette) ButtonFocusedStyle() cell.Style {
	return cell.Style{Fg: p.BgCanvas, Bg: p.AccentPurple, Modifier: cell.ModifierBold}
}
