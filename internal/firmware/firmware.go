// Package firmware implements read-only firmware update checks.
//
// The package compares the firmware version a device reports with metadata
// published by its vendor and explains how to update with the vendor's
// official tools. It never downloads firmware images, never enters a
// bootloader or update mode, and never writes anything to a controller.
//
// Vendor knowledge lives in Providers (see firmware/sony); the Service is
// independent of drivers and of the UI.
package firmware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
)

// Status is the outcome of one check.
type Status string

const (
	// StatusUpdateAvailable: a newer firmware is published for this device
	// (verified against every firmware target the device can use).
	StatusUpdateAvailable Status = "update_available"
	// StatusUpToDate: the installed firmware is at or above every published
	// version the device can use.
	StatusUpToDate Status = "up_to_date"
	// StatusUndetermined: the comparison cannot be made confidently.
	StatusUndetermined Status = "undetermined"
	// StatusUnsupported: no firmware check exists for this model/revision.
	StatusUnsupported Status = "unsupported"
	// StatusNetworkUnavailable: the metadata server could not be reached.
	StatusNetworkUnavailable Status = "network_unavailable"
	// StatusSourceUnavailable: the server answered, but not with usable metadata.
	StatusSourceUnavailable Status = "source_unavailable"
)

// Subject is everything a provider may use to identify a device. It is a
// snapshot, so checks can run off the UI goroutine.
type Subject struct {
	Key        string // session device key
	Descriptor device.Descriptor
	Info       device.Info
	Caps       device.CapabilitySet
	// Identity is what the driver read from the device; HasIdentity is false
	// when the driver does not implement device.FirmwareIdentityProvider.
	Identity    device.FirmwareIdentity
	HasIdentity bool
	Simulated   bool
}

// NewSubject snapshots a device for a check.
func NewSubject(key string, dev device.Device, simulated bool) Subject {
	if dev == nil {
		return Subject{}
	}
	sub := Subject{Key: key, Descriptor: dev.Descriptor(), Info: dev.Info(), Caps: dev.Capabilities(), Simulated: simulated}
	if p, ok := dev.(device.FirmwareIdentityProvider); ok {
		sub.Identity, sub.HasIdentity = p.FirmwareIdentity(), true
	}
	return sub
}

// StoreKey identifies the physical controller across sessions: vendor and
// product ID plus the MAC address when known. Simulated devices never share
// a key with hardware.
func (s Subject) StoreKey() string {
	if s.Simulated {
		return "simulated:" + s.Key
	}
	k := fmt.Sprintf("%04x:%04x", s.Info.VendorID, s.Info.ProductID)
	if mac := strings.ToUpper(strings.TrimSpace(s.Info.MACAddress)); mac != "" {
		k += ":" + mac
	}
	return k
}

// Candidate is one firmware target a device may use, with its published version.
type Candidate struct {
	Target string `json:"target"`
	Latest string `json:"latest,omitempty"` // empty when missing from the metadata
}

// Result is the outcome of one check, ready for display.
type Result struct {
	Key      string `json:"-"`
	StoreKey string `json:"-"`
	Status   Status `json:"status"`
	Provider string `json:"provider,omitempty"`
	Source   string `json:"source,omitempty"` // metadata URL

	Installed string `json:"installed,omitempty"` // installed comparable version
	// Available is the published version to show: exact, or a range when the
	// device's firmware target is not known but every target is newer.
	Available string `json:"available,omitempty"`
	// Target is the identified firmware target (only when verified).
	Target     string      `json:"target,omitempty"`
	Candidates []Candidate `json:"candidates,omitempty"`

	Summary     string   `json:"-"` // one plain-language sentence
	Basis       string   `json:"-"` // how the conclusion was reached
	Diagnostics []string `json:"-"` // technical details for Advanced mode

	// CatalogOK is true when valid metadata was fetched and evaluated (the
	// check "succeeded", even if the comparison itself was inconclusive).
	CatalogOK bool      `json:"-"`
	CheckedAt time.Time `json:"checked_at"`
	Simulated bool      `json:"-"`
	// Connection is the transport the device used when it was checked; a
	// result is discarded when the controller reconnects differently.
	Connection device.ConnectionType `json:"-"`
	// Notify is set by the Service when the user has not yet been told about
	// this available version.
	Notify bool `json:"-"`
}

// Verified reports whether the result is a definite comparison.
func (r Result) Verified() bool {
	return r.Status == StatusUpdateAvailable || r.Status == StatusUpToDate
}

// Link is an official web page.
type Link struct {
	Label string
	URL   string
}

// GuideSection is a titled list of steps.
type GuideSection struct {
	Title string
	Steps []string
}

// Guide explains how to update with the vendor's official tools.
type Guide struct {
	Summary      string
	PlatformNote string // shown prominently (e.g. no Linux updater exists)
	Links        []Link
	Sections     []GuideSection
	Caution      string
}

// Provider checks one vendor's devices against that vendor's metadata.
type Provider interface {
	ID() string
	Name() string
	// Handles reports whether the provider knows this device family.
	Handles(Subject) bool
	// InstalledVersion formats the version read from the device ("" if unknown).
	InstalledVersion(Subject) string
	// Check performs the read-only comparison. It must honor ctx.
	Check(ctx context.Context, sub Subject) Result
	// Guide returns official update instructions for the host OS (runtime.GOOS value).
	Guide(goos string) Guide
}
