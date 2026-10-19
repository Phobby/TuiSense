package audio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ALSA is the Linux backend. It discovers cards via /proc/asound and streams
// through alsa-utils (arecord/aplay) using an explicit "plughw:CARD=…" PCM so
// audio can never fall through to the system default device. When PipeWire or
// PulseAudio currently holds the card, the open fails with "busy" and the error
// is surfaced to the user rather than silently re-routed.
type ALSA struct {
	ProcRoot string // default "/proc/asound" (overridable in tests)
	Arecord  string // default "arecord"
	Aplay    string // default "aplay"
}

// NewALSA returns the Linux ALSA backend with default paths.
func NewALSA() *ALSA {
	return &ALSA{ProcRoot: "/proc/asound", Arecord: "arecord", Aplay: "aplay"}
}

func (a *ALSA) Name() string { return "ALSA" }

var cardLine = regexp.MustCompile(`^\s*(\d+)\s+\[([^\]]+)\]:\s*(.*)$`)

// Endpoints parses /proc/asound/cards plus each card's usbid and stream0 files.
func (a *ALSA) Endpoints() ([]Endpoint, error) {
	data, err := os.ReadFile(filepath.Join(a.ProcRoot, "cards"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Endpoint
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		m := cardLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		idx, _ := strconv.Atoi(m[1])
		id := strings.TrimSpace(m[2])
		name := m[3]
		if i := strings.Index(name, " - "); i >= 0 {
			name = strings.TrimSpace(name[i+3:])
		}
		cardDir := filepath.Join(a.ProcRoot, fmt.Sprintf("card%d", idx))
		ep := Endpoint{
			Backend:   "alsa",
			CardIndex: idx,
			CardID:    id,
			CardName:  name,
			USBID:     strings.ToLower(strings.TrimSpace(readFile(filepath.Join(cardDir, "usbid")))),
			PCM:       fmt.Sprintf("plughw:CARD=%s,DEV=0", id),
		}
		ep.PlaybackChannels, ep.CaptureChannels = parseStreamChannels(readFile(filepath.Join(cardDir, "stream0")))
		if ep.USBID == "" {
			// Non-USB cards: infer directions from PCM device nodes.
			ep.PlaybackChannels, ep.CaptureChannels = probePCMDirs(cardDir)
		}
		out = append(out, ep)
	}
	return out, nil
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// parseStreamChannels reads "Channels: N" under the Playback: and Capture:
// sections of a USB-audio stream0 file.
func parseStreamChannels(stream string) (playback, capture int) {
	section := ""
	for _, line := range strings.Split(stream, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "Playback:"):
			section = "p"
		case strings.HasPrefix(t, "Capture:"):
			section = "c"
		case strings.HasPrefix(t, "Channels:"):
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(t, "Channels:")))
			if section == "p" && playback == 0 {
				playback = n
			} else if section == "c" && capture == 0 {
				capture = n
			}
		}
	}
	return playback, capture
}

func probePCMDirs(cardDir string) (playback, capture int) {
	if _, err := os.Stat(filepath.Join(cardDir, "pcm0p")); err == nil {
		playback = 2
	}
	if _, err := os.Stat(filepath.Join(cardDir, "pcm0c")); err == nil {
		capture = 2
	}
	return playback, capture
}

func toolError(tool string, err error, stderr *bytes.Buffer) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%s not found — install alsa-utils", tool)
	}
	msg := strings.TrimSpace(stderr.String())
	switch {
	case strings.Contains(msg, "busy"):
		return fmt.Errorf("audio device is busy (held by PipeWire/PulseAudio or another app): %s", msg)
	case strings.Contains(strings.ToLower(msg), "permission"):
		return fmt.Errorf("permission denied opening the audio device (is your user in the 'audio' group?): %s", msg)
	case msg != "":
		return fmt.Errorf("%s: %s", tool, msg)
	}
	return fmt.Errorf("%s: %w", tool, err)
}

// Capture runs arecord on the endpoint and delivers ~50 ms mono chunks.
func (a *ALSA) Capture(ctx context.Context, ep Endpoint, rate int, onChunk func([]int16)) error {
	if !ep.CanCapture() {
		return fmt.Errorf("%s has no capture stream", ep.CardName)
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, a.Arecord, "-q", "-D", ep.PCM, "-f", "S16_LE", "-r", strconv.Itoa(rate), "-c", "1", "-t", "raw")
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return toolError("arecord", err, &stderr)
	}
	buf := make([]byte, rate/20*2)
	for {
		if _, err := io.ReadFull(stdout, buf); err != nil {
			werr := cmd.Wait()
			if ctx.Err() != nil {
				return nil
			}
			if werr != nil {
				return toolError("arecord", werr, &stderr)
			}
			return toolError("arecord", err, &stderr)
		}
		chunk := make([]int16, len(buf)/2)
		for i := range chunk {
			chunk[i] = int16(binary.LittleEndian.Uint16(buf[2*i:]))
		}
		onChunk(chunk)
	}
}

// Play pipes interleaved PCM to aplay on the endpoint.
func (a *ALSA) Play(ctx context.Context, ep Endpoint, pcm []int16, rate, channels int) error {
	if !ep.CanPlay() {
		return fmt.Errorf("%s has no playback stream", ep.CardName)
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, a.Aplay, "-q", "-D", ep.PCM, "-f", "S16_LE", "-r", strconv.Itoa(rate), "-c", strconv.Itoa(channels), "-t", "raw")
	cmd.Stderr = &stderr
	raw := make([]byte, len(pcm)*2)
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(raw[2*i:], uint16(s))
	}
	cmd.Stdin = bytes.NewReader(raw)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return toolError("aplay", err, &stderr)
	}
	return nil
}
