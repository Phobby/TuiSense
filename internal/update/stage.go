package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxArchive = 128 << 20
	maxBinary  = 256 << 20
	stateFile  = "staged.json"
)

// Staged is a downloaded, verified update waiting for the next restart.
type Staged struct {
	Version       string    `json:"version"`
	Artifact      string    `json:"artifact"`
	ArchiveSHA256 string    `json:"archive_sha256"`
	BinarySHA256  string    `json:"binary_sha256"`
	Binary        string    `json:"binary"`
	GOOS          string    `json:"goos"`
	GOARCH        string    `json:"goarch"`
	Signer        string    `json:"signer"`
	StagedAt      time.Time `json:"staged_at"`
}

// BinaryName is the executable name on goos.
func BinaryName(goos string) string {
	if goos == "windows" {
		return "tuisense.exe"
	}
	return "tuisense"
}

// ParseSums parses "hex␣␣name" lines (sha256sum format).
func ParseSums(data []byte) (map[string]string, error) {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return nil, fmt.Errorf("malformed checksum line %q", line)
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			return nil, fmt.Errorf("malformed checksum line %q", line)
		}
		out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return out, sc.Err()
}

// Stage downloads the package for goos/goarch from rel, verifies the signed
// checksum list against the pinned keys and the package against it, extracts
// the executable and records it under dir. Nothing is executed. On any
// failure the partial files are removed and the current installation is
// untouched.
func (s Source) Stage(ctx context.Context, rel Release, dir, goos, goarch string, keys []TrustedKey, progress func(done, total int64)) (st Staged, err error) {
	if len(keys) == 0 {
		return Staged{}, ErrNoTrustedKey
	}
	sumsAsset, ok1 := rel.Asset(SumsName)
	sigAsset, ok2 := rel.Asset(SigName)
	if !ok1 || !ok2 {
		return Staged{}, fmt.Errorf("release %s has no signed checksum list", rel.Tag)
	}
	var sums, sig bytes.Buffer
	if err := s.fetch(ctx, sumsAsset.URL, 64<<10, &sums, nil); err != nil {
		return Staged{}, fmt.Errorf("checksums: %w", err)
	}
	if err := s.fetch(ctx, sigAsset.URL, 16<<10, &sig, nil); err != nil {
		return Staged{}, fmt.Errorf("signature: %w", err)
	}
	signer, err := VerifySSHSig(sums.Bytes(), sig.Bytes(), keys)
	if err != nil {
		return Staged{}, err
	}
	table, err := ParseSums(sums.Bytes())
	if err != nil {
		return Staged{}, err
	}
	name := ArchiveName(rel.Version, goos, goarch)
	want, ok := table[name]
	asset, ok2 := rel.Asset(name)
	if !ok || !ok2 {
		return Staged{}, fmt.Errorf("%w (%s/%s)", ErrNoAsset, goos, goarch)
	}

	verDir := filepath.Join(dir, rel.Version.String())
	if err := os.MkdirAll(verDir, 0o700); err != nil {
		return Staged{}, err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(verDir)
		}
	}()
	partial := filepath.Join(verDir, name+".partial")
	f, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return Staged{}, err
	}
	h := sha256.New()
	err = s.fetch(ctx, asset.URL, maxArchive, io.MultiWriter(f, h), progress)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Staged{}, fmt.Errorf("download interrupted: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return Staged{}, ErrChecksum
	}
	bin := filepath.Join(verDir, BinaryName(goos))
	binSum, err := extract(partial, name, BinaryName(goos), bin)
	if err != nil {
		return Staged{}, err
	}
	_ = os.Remove(partial)
	st = Staged{Version: rel.Version.String(), Artifact: name, ArchiveSHA256: got, BinarySHA256: binSum, Binary: bin,
		GOOS: goos, GOARCH: goarch, Signer: signer.Name, StagedAt: time.Now().UTC()}
	if err := writeJSON(filepath.Join(dir, stateFile), st); err != nil {
		return Staged{}, err
	}
	// Only one staged update is kept.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() && e.Name() != rel.Version.String() {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
	return st, nil
}

// extract copies the single executable entry out of a tar.gz or zip archive.
func extract(archive, name, binName, dest string) (string, error) {
	var src io.Reader
	var closer func() error
	switch {
	case strings.HasSuffix(name, ".zip"):
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return "", err
		}
		closer = zr.Close
		for _, f := range zr.File {
			if path.Base(f.Name) == binName && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					_ = closer()
					return "", err
				}
				src, closer = rc, func() error { rc.Close(); return zr.Close() }
				break
			}
		}
	default:
		f, err := os.Open(archive)
		if err != nil {
			return "", err
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return "", err
		}
		closer = func() error { gz.Close(); return f.Close() }
		tr := tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = closer()
				return "", err
			}
			if hdr.Typeflag == tar.TypeReg && path.Base(hdr.Name) == binName {
				src = tr
				break
			}
		}
	}
	defer func() { _ = closer() }()
	if src == nil {
		return "", fmt.Errorf("package does not contain %s", binName)
	}
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(src, maxBinary+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxBinary {
		err = errors.New("executable is larger than allowed")
	}
	if err == nil {
		err = os.Rename(tmp, dest)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// LoadStaged returns the staged update if it is complete and intact.
func LoadStaged(dir string) (Staged, bool) {
	var st Staged
	b, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil || json.Unmarshal(b, &st) != nil {
		return Staged{}, false
	}
	if sum, err := fileSHA256(st.Binary); err != nil || sum != st.BinarySHA256 {
		return Staged{}, false
	}
	return st, true
}

// Discard removes any staged update.
func Discard(dir string) error {
	if err := os.Remove(filepath.Join(dir, stateFile)); err != nil && !os.IsNotExist(err) {
		return err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeJSON(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
