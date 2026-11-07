//go:build darwin && cgo

package dualsense

import (
	"context"
	"os"
	"testing"
	"time"
)

// Manual check with a real controller: TUISENSE_HW=1 go test -run TestMacHardware -v ./internal/drivers/dualsense
func TestMacHardware(t *testing.T) {
	if os.Getenv("TUISENSE_HW") != "1" {
		t.Skip("set TUISENSE_HW=1 with a DualSense connected")
	}
	devs, err := scanDevices()
	if err != nil || len(devs) == 0 {
		t.Fatalf("no DualSense found: %v", err)
	}
	t.Logf("found %+v", devs[0])
	d, err := openBackend(devs[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	b := d.(*HIDBackend)
	t.Logf("info: conn=%v perm=%v hint=%q fw=%+v", b.info.Connection, b.info.PermissionDenied, b.info.PermissionHint, b.fwIdentity)
	deadline := time.Now().Add(3 * time.Second)
	reports := 0
	for time.Now().Before(deadline) {
		tel, err := d.Poll(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		reports += len(tel.Samples)
		time.Sleep(40 * time.Millisecond)
	}
	tel := d.Telemetry()
	t.Logf("reports=%d readErrors=%d battery=%d%% LX=%.2f buttons=%v", reports, tel.ReadErrors, tel.BatteryPercent, tel.LeftStickX, tel.Buttons)
	if reports == 0 {
		t.Fatal("no input reports parsed")
	}
	cfg := d.CurrentConfig()
	cfg.Lighting.Enabled, cfg.Lighting.Red, cfg.Lighting.Green, cfg.Lighting.Blue = true, 0, 200, 0
	if err := d.ApplyConfig(context.Background(), cfg); err != nil {
		t.Fatalf("output report: %v", err)
	}
	t.Log("light bar set to green")
	time.Sleep(time.Second)
}
