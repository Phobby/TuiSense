package sony

import (
	"github.com/Phobby/TuiSense/internal/firmware"
	"github.com/Phobby/TuiSense/internal/i18n"
)

// Official Sony pages. Opening them only shows a web page; it never updates
// the controller.
const (
	PCAccessoriesURL = "https://www.playstation.com/tr-tr/pc/pc-accessories/"
	SupportURL       = "https://www.playstation.com/tr-tr/support/hardware/dualsense-controller-support/"
)

// OfficialLinks are the only URLs the firmware panel offers to open or copy.
var OfficialLinks = []string{PCAccessoriesURL, SupportURL}

// Guide implements firmware.Provider.
func (p *Provider) Guide(goos string) firmware.Guide {
	g := firmware.Guide{
		Summary: i18n.T("Sony currently documents two official ways to update DualSense firmware: PlayStation Accessories on Windows, or a PS5 console."),
		Links: []firmware.Link{
			{Label: i18n.T("PlayStation Accessories (Windows)"), URL: PCAccessoriesURL},
			{Label: i18n.T("DualSense support & update instructions"), URL: SupportURL},
		},
		Sections: []firmware.GuideSection{
			{Title: i18n.T("Windows PC"), Steps: []string{
				i18n.T("On a Windows PC, install PlayStation Accessories from Sony's PC accessories page."),
				i18n.T("Connect the controller with a USB cable and make sure it is charged."),
				i18n.T("Open PlayStation Accessories and follow its instructions. Do not disconnect the controller until it finishes."),
			}},
			{Title: i18n.T("PS5 console"), Steps: []string{
				i18n.T("Connect the controller to the PS5 (a USB cable is recommended)."),
				i18n.T("Open Settings › Accessories › Controllers (General) › Wireless Controller Device Software (the label can differ slightly between PS5 system versions)."),
				i18n.T("Follow the on-screen instructions. The PS5 also offers updates automatically when one is available."),
			}},
		},
		Caution: i18n.T("Any firmware update carries a small risk. Keep the controller charged and connected until the official updater finishes. TuiSense never installs firmware itself."),
	}
	switch goos {
	case "linux":
		g.PlatformNote = i18n.T("Sony does not currently provide an official Linux or browser-based DualSense firmware updater. To update safely, use PlayStation Accessories on Windows or a PS5 console.")
	case "windows":
		g.PlatformNote = i18n.T("Use Sony's PlayStation Accessories app on this PC to install the update.")
	default:
		g.PlatformNote = i18n.T("Sony does not provide an official firmware updater for this operating system. Use PlayStation Accessories on Windows or a PS5 console.")
	}
	return g
}
