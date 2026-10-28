// Command release packages TuiSense release artifacts in the exact layout the
// installers and the updater expect. It is used by the release workflow on
// every native runner and by the installer tests.
//
//	go run ./tools/release package -version 0.8.0 -os linux -arch amd64 -bin tuisense -out dist
//	go run ./tools/release sums -out dist            # writes dist/SHA256SUMS
//	go run ./tools/release verify -out dist -keys internal/update/trusted_keys.txt
//
// Signing itself is done with OpenSSH in CI:
//
//	ssh-keygen -Y sign -f release_key -n tuisense-release dist/SHA256SUMS
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Phobby/TuiSense/internal/update"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: release package|sums|verify [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "package":
		err = packageCmd(os.Args[2:])
	case "sums":
		err = sumsCmd(os.Args[2:])
	case "verify":
		err = verifyCmd(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}

func packageCmd(args []string) error {
	fs := flag.NewFlagSet("package", flag.ExitOnError)
	ver := fs.String("version", "", "version without v")
	goos := fs.String("os", "", "GOOS")
	goarch := fs.String("arch", "", "GOARCH")
	bin := fs.String("bin", "", "built executable")
	out := fs.String("out", "dist", "output directory")
	extra := fs.String("files", "LICENSE,README.md", "comma-separated files to include")
	_ = fs.Parse(args)
	v, err := update.ParseSemver(*ver)
	if err != nil {
		return err
	}
	if *goos == "" || *goarch == "" || *bin == "" {
		return fmt.Errorf("-os, -arch and -bin are required")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	name := update.ArchiveName(v, *goos, *goarch)
	dir := strings.TrimSuffix(strings.TrimSuffix(name, ".tar.gz"), ".zip")
	files := map[string]string{dir + "/" + update.BinaryName(*goos): *bin}
	for _, f := range strings.Split(*extra, ",") {
		if f = strings.TrimSpace(f); f != "" {
			files[dir+"/"+filepath.Base(f)] = f
		}
	}
	path := filepath.Join(*out, name)
	if *goos == "windows" {
		err = writeZip(path, files)
	} else {
		err = writeTarGz(path, files)
	}
	if err == nil {
		fmt.Println(path)
	}
	return err
}

func sortedKeys(m map[string]string) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Fixed timestamps keep archives reproducible.
var epoch = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

func writeTarGz(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, name := range sortedKeys(files) {
		data, err := os.ReadFile(files[name])
		if err != nil {
			return err
		}
		mode := int64(0o644)
		if strings.HasSuffix(name, "/tuisense") {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data)), ModTime: epoch, Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err := tw.Write(data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeZip(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for _, name := range sortedKeys(files) {
		data, err := os.ReadFile(files[name])
		if err != nil {
			return err
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: epoch})
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return zw.Close()
}

func sumsCmd(args []string) error {
	fs := flag.NewFlagSet("sums", flag.ExitOnError)
	out := fs.String("out", "dist", "directory with the archives")
	_ = fs.Parse(args)
	matches, _ := filepath.Glob(filepath.Join(*out, "tuisense_*"))
	sort.Strings(matches)
	if len(matches) == 0 {
		return fmt.Errorf("no archives in %s", *out)
	}
	var b strings.Builder
	for _, m := range matches {
		f, err := os.Open(m)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(h.Sum(nil)), filepath.Base(m))
	}
	return os.WriteFile(filepath.Join(*out, update.SumsName), []byte(b.String()), 0o644)
}

// verifyCmd checks SHA256SUMS.sig against the pinned keys and every archive
// against SHA256SUMS, exactly as the updater will.
func verifyCmd(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	out := fs.String("out", "dist", "release directory")
	keysFile := fs.String("keys", "internal/update/trusted_keys.txt", "pinned keys")
	_ = fs.Parse(args)
	keyText, err := os.ReadFile(*keysFile)
	if err != nil {
		return err
	}
	keys, err := update.ParseTrustedKeys(string(keyText))
	if err != nil {
		return err
	}
	sums, err := os.ReadFile(filepath.Join(*out, update.SumsName))
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(filepath.Join(*out, update.SigName))
	if err != nil {
		return err
	}
	signer, err := update.VerifySSHSig(sums, sig, keys)
	if err != nil {
		return err
	}
	table, err := update.ParseSums(sums)
	if err != nil {
		return err
	}
	for name, want := range table {
		data, err := os.ReadFile(filepath.Join(*out, name))
		if err != nil {
			return err
		}
		h := sha256.Sum256(data)
		if hex.EncodeToString(h[:]) != want {
			return fmt.Errorf("%s: checksum mismatch", name)
		}
	}
	fmt.Printf("verified %d archives, signed by %s\n", len(table), signer.Name)
	return nil
}
