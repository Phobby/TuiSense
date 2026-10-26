package sony

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/firmware"
)

const (
	pidDualSense = 0x0CE6
	pidEdge      = 0x0DF2
)

func subject(pid uint16, update uint32) firmware.Subject {
	return firmware.Subject{
		Key:  "hw:test",
		Info: device.Info{VendorID: sonyVendorID, ProductID: pid, Connected: true, MACAddress: "AA:BB:CC:DD:EE:FF"},
		Caps: device.NewCapabilitySet(device.CapabilitySpec{ID: device.CapFirmwareCheck, Support: device.Full}),
		Identity: device.FirmwareIdentity{
			Read: true, Source: "test", UpdateVersion: update, HasUpdateVersion: update != 0,
		},
		HasIdentity: true,
	}
}

func catalogFetcher(body string) Fetcher {
	return FetcherFunc(func(context.Context) (Catalog, error) {
		cat, err := ParseCatalog([]byte(body))
		cat.URL = "https://example.test/info.json"
		return cat, err
	})
}

func check(t *testing.T, f Fetcher, sub firmware.Subject) firmware.Result {
	t.Helper()
	return New(f).Check(context.Background(), sub)
}

func TestEdgeVerifiedComparison(t *testing.T) {
	old := check(t, catalogFetcher(liveSample), subject(pidEdge, 0x0154))
	if old.Status != firmware.StatusUpdateAvailable || old.Target != "0044" || old.Installed != "0x0154" || old.Available != "0x0217" {
		t.Fatalf("outdated Edge: %+v", old)
	}
	if !old.CatalogOK || !old.Verified() {
		t.Fatal("a single-target comparison is a verified result")
	}
	cur := check(t, catalogFetcher(liveSample), subject(pidEdge, 0x0217))
	if cur.Status != firmware.StatusUpToDate || cur.Available != "0x0217" {
		t.Fatalf("current Edge: %+v", cur)
	}
	newer := check(t, catalogFetcher(liveSample), subject(pidEdge, 0x0300))
	if newer.Status != firmware.StatusUpToDate || len(newer.Diagnostics) == 0 {
		t.Fatalf("firmware newer than the metadata is up to date, with a note: %+v", newer)
	}
}

func TestDualSenseAllTargetsAgree(t *testing.T) {
	// Older than every target's latest: an update exists whichever board this is.
	old := check(t, catalogFetcher(liveSample), subject(pidDualSense, 0x0458))
	if old.Status != firmware.StatusUpdateAvailable {
		t.Fatalf("outdated DualSense: %+v", old)
	}
	if old.Target != "" {
		t.Fatal("the DualSense target is not reported by the controller and must not be claimed")
	}
	if old.Available != "0x0630–0x0641" {
		t.Fatalf("available must show the range across targets, got %q", old.Available)
	}
	// At or above every target's latest: current whichever board this is.
	cur := check(t, catalogFetcher(liveSample), subject(pidDualSense, 0x0641))
	if cur.Status != firmware.StatusUpToDate || cur.Target != "" {
		t.Fatalf("current DualSense: %+v", cur)
	}
}

func TestDualSenseUnknownRevisionIsUndetermined(t *testing.T) {
	// 0x0630 is current for targets 0004/000B but older than 000E's 0x0641.
	r := check(t, catalogFetcher(liveSample), subject(pidDualSense, 0x0630))
	if r.Status != firmware.StatusUndetermined || r.Verified() {
		t.Fatalf("an ambiguous revision must be undetermined: %+v", r)
	}
	if r.Available != "" || r.Target != "" {
		t.Fatalf("no version or target may be claimed: %+v", r)
	}
	if !strings.Contains(r.Basis, "000E") || len(r.Candidates) != 3 {
		t.Fatalf("the explanation should name the conflicting target: %+v", r)
	}
	if !r.CatalogOK {
		t.Fatal("the metadata itself was valid")
	}
}

func TestUnknownSonyProductIsUnsupported(t *testing.T) {
	r := check(t, catalogFetcher(liveSample), subject(0x0CE7, 0x0630))
	if r.Status != firmware.StatusUnsupported {
		t.Fatalf("unknown product: %+v", r)
	}
}

func TestIncorrectTargetMappingIsRefused(t *testing.T) {
	// The Edge's target is missing; other targets must never be used instead.
	body := `{"FwUpdate0004LatestVersion":"0x0630","FwUpdate000BLatestVersion":"0x0630","FwUpdate000ELatestVersion":"0x0641"}`
	r := check(t, catalogFetcher(body), subject(pidEdge, 0x0154))
	if r.Status != firmware.StatusUndetermined || r.Available != "" || r.CatalogOK {
		t.Fatalf("Edge must not be compared with DualSense targets: %+v", r)
	}
	// One of the DualSense's possible targets is missing: no claim either.
	body = `{"FwUpdate0004LatestVersion":"0x0630","FwUpdate0044LatestVersion":"0x0217"}`
	r = check(t, catalogFetcher(body), subject(pidDualSense, 0x0458))
	if r.Status != firmware.StatusUndetermined || r.Available != "" {
		t.Fatalf("incomplete targets must not yield a claim: %+v", r)
	}
}

func TestMissingFirmwareData(t *testing.T) {
	noVersion := subject(pidEdge, 0)
	if r := check(t, catalogFetcher(liveSample), noVersion); r.Status != firmware.StatusUndetermined || r.Installed != "" {
		t.Fatalf("no update version: %+v", r)
	}
	unread := subject(pidEdge, 0x0154)
	unread.Identity = device.FirmwareIdentity{Error: "feature report 0x20 could not be read: permission denied"}
	r := check(t, catalogFetcher(liveSample), unread)
	if r.Status != firmware.StatusUndetermined || len(r.Diagnostics) == 0 || !strings.Contains(r.Diagnostics[0], "permission") {
		t.Fatalf("unreadable identity: %+v", r)
	}
	noIdentity := subject(pidEdge, 0x0154)
	noIdentity.HasIdentity = false
	if r := check(t, catalogFetcher(liveSample), noIdentity); r.Status != firmware.StatusUndetermined {
		t.Fatalf("driver without identity: %+v", r)
	}
	// The device's version is never fetched when it is unknown.
	called := false
	_ = check(t, FetcherFunc(func(context.Context) (Catalog, error) { called = true; return Catalog{}, nil }), noVersion)
	if called {
		t.Fatal("metadata must not be fetched when the installed version is unknown")
	}
}

func TestUnexpectedAPIChanges(t *testing.T) {
	// A new, unrecognized target could belong to this controller.
	body := strings.Replace(liveSample, `"FwUpdate0044`, `"FwUpdate0051LatestVersion": "0x0100", "FwUpdate0044`, 1)
	r := check(t, catalogFetcher(body), subject(pidEdge, 0x0154))
	if r.Status != firmware.StatusUndetermined || r.Available != "" {
		t.Fatalf("unknown targets must block a claim: %+v", r)
	}
	// Renamed keys: no versions at all.
	r = check(t, catalogFetcher(`{"latest":{"dualsense":"0x0630"}}`), subject(pidDualSense, 0x0458))
	if r.Status != firmware.StatusUndetermined || !strings.Contains(r.Summary, "format") {
		t.Fatalf("renamed keys: %+v", r)
	}
	// Values in a new format.
	r = check(t, catalogFetcher(`{"FwUpdate0044LatestVersion":"2.17"}`), subject(pidEdge, 0x0154))
	if r.Status != firmware.StatusUndetermined || len(r.Diagnostics) == 0 {
		t.Fatalf("new value format: %+v", r)
	}
}

func TestFetchFailuresMapToStates(t *testing.T) {
	cases := []struct {
		err  error
		want firmware.Status
	}{
		{&FetchError{Kind: ErrNetwork, Err: errors.New("dial tcp: no route to host")}, firmware.StatusNetworkUnavailable},
		{context.DeadlineExceeded, firmware.StatusNetworkUnavailable},
		{&FetchError{Kind: ErrSource, Err: errors.New("HTTP 503")}, firmware.StatusSourceUnavailable},
		{&FetchError{Kind: ErrMalformed, Err: errors.New("not JSON")}, firmware.StatusSourceUnavailable},
	}
	for _, c := range cases {
		err := c.err
		r := check(t, FetcherFunc(func(context.Context) (Catalog, error) { return Catalog{}, err }), subject(pidEdge, 0x0154))
		if r.Status != c.want || r.CatalogOK || r.Available != "" {
			t.Errorf("%v: got %+v", c.err, r)
		}
	}
	malformed := check(t, catalogFetcher(`{oops`), subject(pidEdge, 0x0154))
	if malformed.Status != firmware.StatusSourceUnavailable {
		t.Fatalf("malformed server response: %+v", malformed)
	}
}

func TestGuideIsOfficialAndHonest(t *testing.T) {
	g := New(nil).Guide("linux")
	if !strings.Contains(g.PlatformNote, "does not currently provide an official Linux") {
		t.Fatalf("Linux note missing: %q", g.PlatformNote)
	}
	for _, l := range g.Links {
		if !strings.HasPrefix(l.URL, "https://www.playstation.com/") {
			t.Fatalf("non-official link %q", l.URL)
		}
	}
	if len(g.Sections) != 2 || g.Caution == "" {
		t.Fatalf("guide %+v", g)
	}
	if strings.Contains(strings.ToLower(g.Caution), "risk-free") {
		t.Fatal("never claim an update is risk-free")
	}
}
