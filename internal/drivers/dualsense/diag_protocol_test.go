package dualsense

import (
	"context"
	"os"
	"syscall"
	"testing"

	"github.com/ygzztrk/tuisense/internal/device"
)

func usbReport(seq byte, b7, b8, b9 byte, lx byte) []byte {
	r := make([]byte, 64)
	r[0] = USBInputReportID
	r[1], r[2], r[3], r[4] = lx, 128, 128, 128
	r[7] = seq
	r[8], r[9], r[10] = b7, b8, b9
	r[33], r[37] = 0x80, 0x80 // no touch
	return r
}

func TestDecodeButtonsMatchesHIDLayout(t *testing.T) {
	set := DecodeButtons(0x20|0x02, 0x01|0x20, 0x01|0x02|0x04) // cross + hat E, L1 + options, PS + touchpad + mic
	want := []device.ButtonID{device.ButtonSouth, device.ButtonDpadRight, device.ButtonL1, device.ButtonMenu,
		device.ButtonHome, device.ButtonTouchpadClick, device.ButtonMicMute}
	for _, id := range want {
		if !set.Has(id) {
			t.Errorf("button %d not decoded", id)
		}
	}
	if set.Count() != len(want) {
		t.Errorf("unexpected extra buttons: %d", set.Count())
	}
	if DecodeButtons(0x08, 0, 0) != 0 {
		t.Errorf("hat value 8 means released")
	}
	for _, s := range []device.ButtonSet{set, device.ButtonSet(0).With(device.ButtonDpadUp).With(device.ButtonDpadLeft).With(device.ButtonR3)} {
		b0, b1, b2 := EncodeButtons(s)
		if DecodeButtons(b0, b1, b2) != s {
			t.Errorf("encode/decode round trip failed for %b", s)
		}
	}
}

func TestParseRawValuesAndTwoTouchPoints(t *testing.T) {
	r := usbReport(0, 0x08, 0, 0, 131)
	r[33] = 0x05                           // finger 1 active, id 5
	r[34], r[35], r[36] = 0x10, 0x34, 0x12 // x = 0x410, y = 0x123
	r[37] = 0x06                           // finger 2 active, id 6
	tel, err := ParseInputReport(r)
	if err != nil {
		t.Fatal(err)
	}
	if !tel.Raw.Valid || tel.Raw.Sticks[0] != 131 {
		t.Fatalf("raw stick value must be preserved without deadzone, got %+v", tel.Raw)
	}
	if !tel.Touch[0].Active || tel.Touch[0].ID != 5 || tel.Touch[0].X != 0x410 || tel.Touch[0].Y != 0x123 {
		t.Fatalf("touch 1 decoded wrong: %+v", tel.Touch[0])
	}
	if !tel.Touch[1].Active || tel.Touch[1].ID != 6 {
		t.Fatalf("second contact not decoded: %+v", tel.Touch[1])
	}
}

// packetPipe returns a non-blocking, message-preserving fd pair (like hidraw).
func packetPipe(t *testing.T) (*os.File, *os.File) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_SEQPACKET, 0)
	if err != nil {
		t.Skipf("socketpair unavailable: %v", err)
	}
	if err := syscall.SetNonblock(fds[0], true); err != nil {
		t.Fatal(err)
	}
	return os.NewFile(uintptr(fds[0]), "read"), os.NewFile(uintptr(fds[1]), "write")
}

func TestPollDrainsQueueLatchesButtonsAndCountsGaps(t *testing.T) {
	r, w := packetPipe(t)
	defer r.Close()
	defer w.Close()
	b := &LinuxBackend{desc: Descriptor(), file: r, info: device.Info{Connected: true, Connection: device.ConnUSB}}

	// 10 queued reports: a quick Cross press in report 3 only, a sequence gap
	// after report 6 (two reports lost), newest stick value 200.
	seqs := []byte{1, 2, 3, 4, 5, 6, 9, 10, 11, 12}
	for i, seq := range seqs {
		b7 := byte(0x08)
		if i == 2 {
			b7 |= 0x20
		}
		lx := byte(128)
		if i == len(seqs)-1 {
			lx = 200
		}
		if _, err := w.Write(usbReport(seq, b7, 0, 0, lx)); err != nil {
			t.Fatal(err)
		}
	}
	tel, err := b.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tel.Raw.Sticks[0] != 200 {
		t.Fatalf("poll must reflect the newest report, got %d", tel.Raw.Sticks[0])
	}
	if !tel.ButtonsLatched.Has(device.ButtonSouth) || tel.Buttons.Has(device.ButtonSouth) {
		t.Fatalf("a press between polls must be latched but not held")
	}
	if tel.PacketsRx != 10 || tel.ReportsDropped != 2 {
		t.Fatalf("expected 10 reports and 2 dropped, got rx=%d dropped=%d", tel.PacketsRx, tel.ReportsDropped)
	}
}
