package firmware_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Phobby/TuiSense/internal/device"
	"github.com/Phobby/TuiSense/internal/drivers/dualsense"
	"github.com/Phobby/TuiSense/internal/drivers/gamepad"
	"github.com/Phobby/TuiSense/internal/drivers/msigc20"
	"github.com/Phobby/TuiSense/internal/drivers/xboxmock"
	"github.com/Phobby/TuiSense/internal/firmware"
	"github.com/Phobby/TuiSense/internal/firmware/sony"
	"github.com/Phobby/TuiSense/internal/system/evdev"
)

func catalog(edge string) sony.Fetcher {
	return sony.FetcherFunc(func(context.Context) (sony.Catalog, error) {
		return sony.ParseCatalog([]byte(`{"FwUpdate0004LatestVersion":"0x0630","FwUpdate000BLatestVersion":"0x0630",
			"FwUpdate000ELatestVersion":"0x0641","FwUpdate0044LatestVersion":"` + edge + `"}`))
	})
}

// hwSubject builds a real-hardware-like subject from the mock DualSense.
func hwSubject(key string, pid uint16, mac string, update uint32) firmware.Subject {
	m := dualsense.NewMockBackend(nil)
	sub := firmware.NewSubject(key, m, false)
	sub.Info.ProductID, sub.Info.MACAddress = pid, mac
	sub.Identity = device.FirmwareIdentity{Read: true, UpdateVersion: update, HasUpdateVersion: true}
	return sub
}

func TestNoConnectedController(t *testing.T) {
	svc := firmware.NewService(nil, sony.New(catalog("0x0217")))
	sub := firmware.NewSubject("", nil, false)
	if svc.Supported(sub) {
		t.Fatal("nothing is supported without a controller")
	}
	r := svc.Check(context.Background(), sub)
	if r.Status != firmware.StatusUndetermined || r.Verified() {
		t.Fatalf("no controller: %+v", r)
	}
}

func TestOnlyDevicesWithARealImplementationAreSupported(t *testing.T) {
	svc := firmware.NewService(nil, sony.New(catalog("0x0217")))
	if !svc.Supported(firmware.NewSubject("mock", dualsense.NewMockBackend(nil), true)) {
		t.Fatal("the DualSense declares a firmware check")
	}
	if svc.Supported(firmware.NewSubject("xbox", xboxmock.New(nil), true)) {
		t.Fatal("the Xbox test device has no firmware check")
	}
	// MSI Force GC20 V2: no capability, no provider.
	desc := msigc20.Describe(gamepad.Mapping{}, evdev.InputDevice{Vendor: 0x0db0, Product: 0x1234}, false)
	sub := firmware.Subject{Key: "msi", Descriptor: desc, Caps: desc.Capabilities,
		Info: device.Info{VendorID: 0x0db0, ProductID: 0x1234, Connected: true}}
	if svc.Supported(sub) {
		t.Fatal("firmware checking must not be advertised for the MSI Force GC20 V2")
	}
	if r := svc.Check(context.Background(), sub); r.Status != firmware.StatusUnsupported {
		t.Fatalf("MSI: %+v", r)
	}
	// A Sony device that does not declare the capability stays unsupported.
	sub = hwSubject("ds", 0x0CE6, "", 0x0458)
	sub.Caps = device.NewCapabilitySet()
	if r := svc.Check(context.Background(), sub); r.Status != firmware.StatusUnsupported {
		t.Fatalf("undeclared capability: %+v", r)
	}
}

func TestDisconnectedControllerIsNotChecked(t *testing.T) {
	var calls atomic.Int32
	f := sony.FetcherFunc(func(ctx context.Context) (sony.Catalog, error) { calls.Add(1); return catalog("0x0217").Fetch(ctx) })
	svc := firmware.NewService(nil, sony.New(f))
	sub := hwSubject("edge", 0x0DF2, "AA:AA:AA:AA:AA:01", 0x0154)
	sub.Info.Connected = false
	if r := svc.Check(context.Background(), sub); r.Status != firmware.StatusUndetermined || calls.Load() != 0 {
		t.Fatalf("disconnected: %+v (fetches %d)", r, calls.Load())
	}
}

func TestPersistenceAndSingleNotification(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	edgeLatest := "0x0217"
	f := sony.FetcherFunc(func(ctx context.Context) (sony.Catalog, error) { return catalog(edgeLatest).Fetch(ctx) })
	svc := firmware.NewService(firmware.NewStore(dir), sony.New(f))
	svc.Now = func() time.Time { return now }
	sub := hwSubject("edge", 0x0DF2, "aa:bb:cc:dd:ee:01", 0x0154)

	if !svc.Due(sub, 24*time.Hour) {
		t.Fatal("a never-checked controller is due")
	}
	r := svc.Check(context.Background(), sub)
	if r.Status != firmware.StatusUpdateAvailable || !r.Notify {
		t.Fatalf("first check must notify: %+v", r)
	}
	if r2 := svc.Check(context.Background(), sub); r2.Notify {
		t.Fatal("the same version must not be announced twice")
	}

	// A fresh service (new session) reads the persisted record.
	svc2 := firmware.NewService(firmware.NewStore(dir), sony.New(f))
	svc2.Now = func() time.Time { return now.Add(time.Hour) }
	rec, ok := svc2.LastSuccess(sub)
	if !ok || rec.Status != firmware.StatusUpdateAvailable || rec.Installed != "0x0154" || rec.Available != "0x0217" || !rec.CheckedAt.Equal(now) {
		t.Fatalf("persisted record %+v ok=%v", rec, ok)
	}
	if svc2.Due(sub, 24*time.Hour) {
		t.Fatal("checked an hour ago: not due")
	}
	if r := svc2.Check(context.Background(), sub); r.Notify {
		t.Fatal("already notified in an earlier session")
	}
	// Sony publishes a newer version: notify again, once.
	edgeLatest = "0x0230"
	if r := svc2.Check(context.Background(), sub); !r.Notify || r.Available != "0x0230" {
		t.Fatalf("a new version must be announced: %+v", r)
	}
}

func TestFailedChecksAreNotRecorded(t *testing.T) {
	svc := firmware.NewService(firmware.NewStore(t.TempDir()), sony.New(sony.FetcherFunc(func(context.Context) (sony.Catalog, error) {
		return sony.Catalog{}, &sony.FetchError{Kind: sony.ErrNetwork, Err: context.DeadlineExceeded}
	})))
	sub := hwSubject("edge", 0x0DF2, "AA:AA:AA:AA:AA:02", 0x0154)
	if r := svc.Check(context.Background(), sub); r.Status != firmware.StatusNetworkUnavailable {
		t.Fatalf("offline: %+v", r)
	}
	if _, ok := svc.LastSuccess(sub); ok {
		t.Fatal("only successful checks are persisted")
	}
}

func TestSimulatedDevicesNeverNotify(t *testing.T) {
	svc := firmware.NewService(nil, sony.New(catalog("0x0217")))
	m := dualsense.NewMockBackend(nil)
	m.SetFirmwareIdentity(device.FirmwareIdentity{Read: true, UpdateVersion: 0x0458, HasUpdateVersion: true})
	r := svc.Check(context.Background(), firmware.NewSubject("mock:dualsense", m, true))
	if r.Status != firmware.StatusUpdateAvailable || r.Notify || !r.Simulated {
		t.Fatalf("simulated: %+v", r)
	}
}

func TestNetworkTimeoutIsBounded(t *testing.T) {
	blocking := sony.FetcherFunc(func(ctx context.Context) (sony.Catalog, error) {
		<-ctx.Done()
		return sony.Catalog{}, ctx.Err()
	})
	svc := firmware.NewService(nil, sony.New(blocking))
	svc.Timeout = 50 * time.Millisecond
	start := time.Now()
	r := svc.Check(context.Background(), hwSubject("edge", 0x0DF2, "", 0x0154))
	if r.Status != firmware.StatusNetworkUnavailable {
		t.Fatalf("timeout: %+v", r)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("check did not respect its timeout")
	}
}

func TestMultipleConnectedControllers(t *testing.T) {
	var calls atomic.Int32
	f := sony.FetcherFunc(func(ctx context.Context) (sony.Catalog, error) { calls.Add(1); return catalog("0x0217").Fetch(ctx) })
	svc := firmware.NewService(firmware.NewStore(t.TempDir()), sony.New(f))
	subs := []firmware.Subject{
		hwSubject("hw:edge", 0x0DF2, "AA:AA:AA:AA:AA:01", 0x0154),     // outdated Edge
		hwSubject("hw:ds1", 0x0CE6, "AA:AA:AA:AA:AA:02", 0x0641),      // current DualSense
		hwSubject("hw:ds2", 0x0CE6, "AA:AA:AA:AA:AA:03", 0x0630),      // ambiguous DualSense
		hwSubject("hw:edge-cur", 0x0DF2, "AA:AA:AA:AA:AA:04", 0x0217), // current Edge
	}
	var mu sync.Mutex
	got := map[string]firmware.Result{}
	var wg sync.WaitGroup
	for _, sub := range subs {
		wg.Add(1)
		if !svc.Start(context.Background(), sub, func(r firmware.Result) {
			mu.Lock()
			got[r.Key] = r
			mu.Unlock()
			wg.Done()
		}) {
			t.Fatalf("%s: check did not start", sub.Key)
		}
	}
	wg.Wait()
	want := map[string]firmware.Status{
		"hw:edge": firmware.StatusUpdateAvailable, "hw:ds1": firmware.StatusUpToDate,
		"hw:ds2": firmware.StatusUndetermined, "hw:edge-cur": firmware.StatusUpToDate,
	}
	for k, st := range want {
		if got[k].Status != st {
			t.Errorf("%s: got %s want %s", k, got[k].Status, st)
		}
		if rec, ok := svc.Store().Get(got[k].StoreKey); !ok || rec.Status != st {
			t.Errorf("%s: each controller keeps its own record, got %+v", k, rec)
		}
	}
	if svc.Running("hw:edge") {
		t.Fatal("finished checks must not stay marked as running")
	}
}

func TestStartIgnoresDuplicateRequests(t *testing.T) {
	release := make(chan struct{})
	f := sony.FetcherFunc(func(ctx context.Context) (sony.Catalog, error) { <-release; return catalog("0x0217").Fetch(ctx) })
	svc := firmware.NewService(nil, sony.New(f))
	sub := hwSubject("hw:edge", 0x0DF2, "", 0x0154)
	done := make(chan firmware.Result, 2)
	if !svc.Start(context.Background(), sub, func(r firmware.Result) { done <- r }) {
		t.Fatal("first start")
	}
	if svc.Start(context.Background(), sub, func(r firmware.Result) { done <- r }) {
		t.Fatal("a second check for the same controller must not start while one runs")
	}
	if !svc.Running(sub.Key) {
		t.Fatal("running flag")
	}
	close(release)
	if r := <-done; r.Status != firmware.StatusUpdateAvailable {
		t.Fatalf("result %+v", r)
	}
}
