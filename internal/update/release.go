package update

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Official update source.
const (
	Owner      = "Phobby"
	Repo       = "TuiSense"
	DefaultAPI = "https://api.github.com"
	// SumsName and SigName are the signed release metadata files.
	SumsName = "SHA256SUMS"
	SigName  = "SHA256SUMS.sig"
)

//go:embed trusted_keys.txt
var trustedKeysText string

// Errors reported by the updater.
var (
	ErrNoTrustedKey       = errors.New("this build has no pinned update signing key; updates cannot be verified")
	ErrUntrustedSignature = errors.New("release metadata is not signed by a trusted TuiSense key")
	ErrNoAsset            = errors.New("the release has no package for this system")
	ErrChecksum           = errors.New("downloaded package does not match the signed checksum")
	ErrUntrustedHost      = errors.New("download URL is not an official release location")
)

// TrustedKeys returns the keys pinned into this build.
func TrustedKeys() []TrustedKey {
	keys, err := ParseTrustedKeys(trustedKeysText)
	if err != nil {
		return nil
	}
	return keys
}

// Channel selects which releases are eligible.
type Channel string

const (
	Stable     Channel = "stable"
	Prerelease Channel = "prerelease"
)

// Asset is one downloadable release file.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release is the subset of GitHub's release object the updater uses.
type Release struct {
	Tag        string  `json:"tag_name"`
	Name       string  `json:"name"`
	Notes      string  `json:"body"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Published  string  `json:"published_at"`
	Assets     []Asset `json:"assets"`
	Version    Semver  `json:"-"`
}

// Asset returns the asset named name.
func (r Release) Asset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// ArchiveName is the package for an OS/architecture.
func ArchiveName(v Semver, goos, goarch string) string {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("tuisense_%s_%s_%s%s", v.String(), goos, goarch, ext)
}

// Source talks to the release API.
type Source struct {
	API    string // default DefaultAPI
	Client *http.Client
	// Hosts are the hosts downloads may come from; nil = GitHub's.
	Hosts []string
	// AllowHTTP permits plain HTTP (local test servers only).
	AllowHTTP bool
}

var githubHosts = []string{"github.com", "api.github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com"}

func (s Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	// Default TLS verification; bounded time so offline use never hangs.
	return &http.Client{Timeout: 10 * time.Second}
}

func (s Source) api() string {
	if s.API != "" {
		return strings.TrimRight(s.API, "/")
	}
	return DefaultAPI
}

// checkURL enforces HTTPS and the official hosts.
func (s Source) checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	hosts := s.Hosts
	if hosts == nil {
		hosts = githubHosts
	}
	if u.Scheme != "https" && !(s.AllowHTTP && u.Scheme == "http") {
		return fmt.Errorf("%w: %s", ErrUntrustedHost, raw)
	}
	for _, h := range hosts {
		if u.Hostname() == h {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUntrustedHost, u.Hostname())
}

// Latest returns the newest eligible release for the channel.
func (s Source) Latest(ctx context.Context, ch Channel) (Release, error) {
	endpoint := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30", s.api(), Owner, Repo)
	if err := s.checkURL(endpoint); err != nil {
		return Release{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "tuisense-updater")
	resp, err := s.client().Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		// GitHub answers 404 for a repository that is private or missing.
		return Release{}, fmt.Errorf("no published releases found for %s/%s", Owner, Repo)
	}
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("release API returned %s", resp.Status)
	}
	var all []Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&all); err != nil {
		return Release{}, fmt.Errorf("invalid release metadata: %w", err)
	}
	var best Release
	found := false
	for _, r := range all {
		v, err := ParseSemver(r.Tag)
		if err != nil || r.Draft {
			continue
		}
		if ch != Prerelease && (r.Prerelease || v.Prerelease()) {
			continue
		}
		r.Version = v
		if !found || v.Compare(best.Version) > 0 {
			best, found = r, true
		}
	}
	if !found {
		return Release{}, errors.New("no eligible release found")
	}
	return best, nil
}

// fetch downloads a URL with a size limit, reporting progress.
func (s Source) fetch(ctx context.Context, raw string, limit int64, w io.Writer, progress func(done, total int64)) error {
	if err := s.checkURL(raw); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "tuisense-updater")
	resp, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Redirects are followed by net/http; the final location must be official too.
	if err := s.checkURL(resp.Request.URL.String()); err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", resp.Status)
	}
	if resp.ContentLength > limit {
		return fmt.Errorf("download is larger than the %d byte limit", limit)
	}
	buf := make([]byte, 32*1024)
	var done int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			done += int64(n)
			if done > limit {
				return fmt.Errorf("download is larger than the %d byte limit", limit)
			}
			if _, err := w.Write(buf[:n]); err != nil {
				return err
			}
			if progress != nil {
				progress(done, resp.ContentLength)
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}
