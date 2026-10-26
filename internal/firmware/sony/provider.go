package sony

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Phobby/TuiSense/internal/firmware"
	"github.com/Phobby/TuiSense/internal/i18n"
)

const sonyVendorID = 0x054C

// product lists the firmware targets one USB product ID can use.
//
// Sony publishes one "latest version" per firmware target and does not
// document which target a controller uses. Community reports (e.g. the
// dualsense-updater-rs project) associate the standard DualSense with three
// targets that differ by internal board revision (0004, 000B, 000E) and the
// DualSense Edge with one (0044). The controller does not report its target,
// so for products with several targets a result is only claimed when it holds
// for every one of them.
type product struct {
	Name    string
	Targets []string
}

var products = map[uint16]product{
	0x0CE6: {Name: "DualSense", Targets: []string{"0004", "000B", "000E"}},
	0x0DF2: {Name: "DualSense Edge", Targets: []string{"0044"}},
}

// knownTargets is every target TuiSense can attribute to a product.
func knownTargets() map[string]bool {
	m := map[string]bool{}
	for _, p := range products {
		for _, t := range p.Targets {
			m[t] = true
		}
	}
	return m
}

// Provider checks DualSense controllers against Sony's metadata.
type Provider struct {
	Fetcher Fetcher
}

// New returns a provider using the given fetcher.
func New(f Fetcher) *Provider { return &Provider{Fetcher: f} }

var _ firmware.Provider = (*Provider)(nil)

func (p *Provider) ID() string   { return "sony" }
func (p *Provider) Name() string { return "Sony firmware updater metadata" }

// Handles accepts Sony controllers by USB vendor ID; unknown Sony products
// are handled so they get an explicit "unsupported" answer.
func (p *Provider) Handles(sub firmware.Subject) bool {
	return sub.Info.VendorID == sonyVendorID
}

// FormatVersion renders an update version the way Sony's metadata does.
func FormatVersion(v uint32) string { return fmt.Sprintf("0x%04X", v) }

// InstalledVersion implements firmware.Provider.
func (p *Provider) InstalledVersion(sub firmware.Subject) string {
	if !sub.HasIdentity || !sub.Identity.Read || !sub.Identity.HasUpdateVersion {
		return ""
	}
	return FormatVersion(sub.Identity.UpdateVersion)
}

// Check implements firmware.Provider.
func (p *Provider) Check(ctx context.Context, sub firmware.Subject) firmware.Result {
	r := firmware.Result{Status: firmware.StatusUndetermined, Provider: p.Name(), Source: CatalogURL}
	prod, ok := products[sub.Info.ProductID]
	if !ok {
		r.Status = firmware.StatusUnsupported
		r.Summary = i18n.Tf("No firmware target is known for this Sony controller (product ID %04X).", sub.Info.ProductID)
		r.Basis = i18n.T("TuiSense only compares firmware for models whose firmware targets are known; it does not guess.")
		return r
	}

	// 1. Installed version, as read from the controller.
	switch {
	case !sub.HasIdentity:
		r.Summary = i18n.T("The installed firmware version could not be read.")
		r.Diagnostics = append(r.Diagnostics, i18n.T("The driver does not expose a firmware identity for this device."))
		return r
	case !sub.Identity.Read:
		r.Summary = i18n.T("The installed firmware version could not be read.")
		if sub.Identity.Error != "" {
			r.Diagnostics = append(r.Diagnostics, sub.Identity.Error)
		}
		return r
	case !sub.Identity.HasUpdateVersion:
		r.Summary = i18n.T("The controller did not report a firmware update version.")
		return r
	}
	installed := sub.Identity.UpdateVersion
	r.Installed = FormatVersion(installed)

	// 2. Metadata.
	if p.Fetcher == nil {
		r.Status = firmware.StatusSourceUnavailable
		r.Summary = i18n.T("No firmware metadata source is configured.")
		return r
	}
	cat, err := p.Fetcher.Fetch(ctx)
	if err != nil {
		var fe *FetchError
		kind := ErrSource
		if errors.As(err, &fe) {
			kind = fe.Kind
		} else if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			kind = ErrNetwork
		}
		switch kind {
		case ErrNetwork:
			r.Status = firmware.StatusNetworkUnavailable
			r.Summary = i18n.T("Sony's firmware server could not be reached. Check your internet connection.")
		case ErrMalformed:
			r.Status = firmware.StatusSourceUnavailable
			r.Summary = i18n.T("Sony's firmware server returned data TuiSense does not understand.")
		default:
			r.Status = firmware.StatusSourceUnavailable
			r.Summary = i18n.T("Sony's firmware metadata is currently unavailable.")
		}
		r.Diagnostics = append(r.Diagnostics, err.Error())
		return r
	}
	if cat.URL != "" {
		r.Source = cat.URL
	}
	for _, k := range cat.Invalid {
		r.Diagnostics = append(r.Diagnostics, i18n.Tf("Ignored metadata entry with an invalid value: %s", k))
	}
	if len(cat.Unrecognized) > 0 {
		r.Diagnostics = append(r.Diagnostics, i18n.Tf("Unrecognized metadata fields (ignored): %s", strings.Join(cat.Unrecognized, ", ")))
	}

	// 3. Unknown targets mean the metadata changed in a way TuiSense cannot
	// interpret: one of them could belong to this controller.
	known := knownTargets()
	var unknown []string
	for t := range cat.Targets {
		if !known[t] {
			unknown = append(unknown, t)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		r.Summary = i18n.T("Sony's metadata lists firmware targets TuiSense does not recognize, so it cannot compare safely.")
		r.Diagnostics = append(r.Diagnostics, i18n.Tf("Unrecognized firmware targets: %s", strings.Join(unknown, ", ")))
		return r
	}

	// 4. Every target this product can use must be listed.
	var missing []string
	lo, hi := uint16(0xFFFF), uint16(0)
	for _, t := range prod.Targets {
		v, ok := cat.Targets[t]
		c := firmware.Candidate{Target: t}
		if ok {
			c.Latest = FormatVersion(uint32(v))
			lo, hi = min(lo, v), max(hi, v)
		} else {
			missing = append(missing, t)
		}
		r.Candidates = append(r.Candidates, c)
	}
	if len(missing) > 0 {
		if len(cat.Targets) == 0 {
			r.Summary = i18n.T("Sony's metadata format has changed: no firmware versions were found.")
		} else {
			r.Summary = i18n.Tf("Sony's metadata does not list firmware for this model (target %s).", strings.Join(missing, ", "))
		}
		return r
	}
	r.CatalogOK = true
	single := len(prod.Targets) == 1
	if single {
		r.Target = prod.Targets[0]
	}
	targets := strings.Join(prod.Targets, ", ")

	// 5. Compare.
	switch {
	case uint32(hi) <= installed:
		r.Status = firmware.StatusUpToDate
		if lo == hi {
			r.Available = FormatVersion(uint32(hi))
		}
		r.Summary = i18n.Tf("Firmware %s is up to date.", r.Installed)
		if single {
			r.Basis = i18n.Tf("Compared with the latest version Sony lists for firmware target %s, the only target known for the %s.", r.Target, prod.Name)
		} else {
			r.Basis = i18n.Tf("The installed version is at or above the latest version Sony lists for every firmware target known for the %s (%s), so it is current whichever target this controller uses.", prod.Name, targets)
		}
		if uint32(hi) < installed {
			r.Diagnostics = append(r.Diagnostics, i18n.T("The installed version is newer than Sony's published metadata (the metadata may lag behind)."))
		}
	case installed < uint32(lo):
		r.Status = firmware.StatusUpdateAvailable
		r.Available = FormatVersion(uint32(lo))
		if lo != hi {
			r.Available += "–" + FormatVersion(uint32(hi))
		}
		r.Summary = i18n.Tf("A firmware update is available: %s → %s.", r.Installed, r.Available)
		if single {
			r.Basis = i18n.Tf("Compared with the latest version Sony lists for firmware target %s, the only target known for the %s.", r.Target, prod.Name)
		} else {
			r.Basis = i18n.Tf("The installed version is older than the latest version Sony lists for every firmware target known for the %s (%s). The exact new version depends on the controller's internal revision.", prod.Name, targets)
		}
	default:
		var current, newer []string
		for _, c := range r.Candidates {
			if v := cat.Targets[c.Target]; uint32(v) <= installed {
				current = append(current, c.Target)
			} else {
				newer = append(newer, c.Target+" ("+c.Latest+")")
			}
		}
		r.Summary = i18n.T("TuiSense cannot tell whether an update is available for this controller.")
		r.Basis = i18n.Tf("The %s uses different firmware targets depending on its internal revision, and the controller does not report which one it has. %s is current for target %s but older than the version listed for target %s.",
			prod.Name, r.Installed, strings.Join(current, ", "), strings.Join(newer, ", "))
	}
	return r
}
