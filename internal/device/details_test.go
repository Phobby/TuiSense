package device

import "testing"

type plainDev struct{ Device }

func (plainDev) Descriptor() Descriptor { return Descriptor{DisplayName: "Pad", Brand: "Acme"} }
func (plainDev) Info() Info {
	return Info{VendorID: 0x1234, ProductID: 0x5678, Connection: ConnUSB, InputDevicePath: "/dev/input/event3", KernelDriver: "xpad"}
}

func TestDetailsOfFallsBackToInfo(t *testing.T) {
	ds := DetailsOf(plainDev{})
	got := map[string]string{}
	for _, d := range ds {
		got[d.Key] = d.Value
	}
	if got["model"] != "Pad" || got["manufacturer"] != "Acme" || got["usb_id"] != "1234:5678" || got["hidraw"] != "/dev/input/event3" || got["driver"] != "xpad" {
		t.Fatalf("got %v", got)
	}
	for _, k := range []string{"fw_version", "fw_build", "hw_version"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s was not reported and must not appear", k)
		}
	}
	if DetailsOf(nil) != nil {
		t.Fatal("no device, no details")
	}
}
