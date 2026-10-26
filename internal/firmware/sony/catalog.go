// Package sony checks Sony DualSense firmware against Sony's firmware
// updater metadata. It is strictly read-only: only the small JSON metadata
// file is fetched, never a firmware image.
package sony

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

// CatalogURL is the metadata file used by Sony's "Firmware updater for
// DualSense wireless controller". It is undocumented and may change at any
// time, so every field is validated before use.
const CatalogURL = "https://fwupdater.dl.playstation.net/fwupdater/info.json"

const (
	maxCatalogBytes = 64 << 10
	httpTimeout     = 10 * time.Second
	cacheFor        = 10 * time.Minute
)

var (
	targetKeyRe    = regexp.MustCompile(`^FwUpdate([0-9A-Fa-f]{4})LatestVersion$`)
	versionHexRe   = regexp.MustCompile(`^0[xX][0-9A-Fa-f]{1,4}$`)
	knownNonTarget = map[string]bool{"ApplicationLatestVersion": true}
)

// Catalog is validated metadata.
type Catalog struct {
	URL       string
	FetchedAt time.Time
	// Targets maps a firmware target ("0004") to its latest update version.
	Targets map[string]uint16
	// Invalid lists target keys whose value failed validation.
	Invalid []string
	// Unrecognized lists keys TuiSense does not know (possible format change).
	Unrecognized []string
	// AppVersion is the updater application's version, when present.
	AppVersion string
}

// ErrorKind classifies fetch failures for the result states.
type ErrorKind int

const (
	ErrNetwork   ErrorKind = iota // server not reachable (offline, DNS, timeout)
	ErrSource                     // server reachable but metadata unusable
	ErrMalformed                  // response is not valid metadata JSON
)

// FetchError is returned by fetchers.
type FetchError struct {
	Kind ErrorKind
	Err  error
}

func (e *FetchError) Error() string { return e.Err.Error() }
func (e *FetchError) Unwrap() error { return e.Err }

// Fetcher retrieves the metadata catalog.
type Fetcher interface {
	Fetch(ctx context.Context) (Catalog, error)
}

// FetcherFunc adapts a function (tests, offline mode).
type FetcherFunc func(ctx context.Context) (Catalog, error)

func (f FetcherFunc) Fetch(ctx context.Context) (Catalog, error) { return f(ctx) }

// HTTPFetcher downloads and validates the catalog over HTTPS, caching a
// successful result briefly so several controllers share one request.
type HTTPFetcher struct {
	URL       string
	Client    *http.Client
	UserAgent string

	mu       sync.Mutex
	cached   Catalog
	cachedAt time.Time
}

// NewHTTPFetcher returns a fetcher for CatalogURL.
func NewHTTPFetcher(userAgent string) *HTTPFetcher {
	return &HTTPFetcher{URL: CatalogURL, UserAgent: userAgent}
}

func (h *HTTPFetcher) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return &http.Client{Timeout: httpTimeout}
}

// Fetch implements Fetcher.
func (h *HTTPFetcher) Fetch(ctx context.Context) (Catalog, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.cachedAt.IsZero() && time.Since(h.cachedAt) < cacheFor {
		return h.cached, nil
	}
	u, err := url.Parse(h.URL)
	if err != nil || u.Scheme != "https" {
		return Catalog{}, &FetchError{Kind: ErrSource, Err: fmt.Errorf("refusing non-HTTPS metadata URL %q", h.URL)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Catalog{}, &FetchError{Kind: ErrSource, Err: err}
	}
	req.Header.Set("Accept", "application/json")
	if h.UserAgent != "" {
		req.Header.Set("User-Agent", h.UserAgent)
	}
	base := h.client()
	c := *base
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" {
			return errors.New("refusing redirect to a non-HTTPS URL")
		}
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		return nil
	}
	if c.Timeout == 0 {
		c.Timeout = httpTimeout
	}
	resp, err := c.Do(req)
	if err != nil {
		return Catalog{}, classifyTransportError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Catalog{}, &FetchError{Kind: ErrSource, Err: fmt.Errorf("metadata server answered HTTP %d", resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogBytes+1))
	if err != nil {
		return Catalog{}, classifyTransportError(ctx, err)
	}
	if len(body) > maxCatalogBytes {
		return Catalog{}, &FetchError{Kind: ErrMalformed, Err: fmt.Errorf("metadata response is larger than %d bytes", maxCatalogBytes)}
	}
	cat, err := ParseCatalog(body)
	if err != nil {
		return Catalog{}, err
	}
	cat.URL, cat.FetchedAt = h.URL, time.Now()
	h.cached, h.cachedAt = cat, time.Now()
	return cat, nil
}

func classifyTransportError(ctx context.Context, err error) error {
	var ne net.Error
	var dnsErr *net.DNSError
	var opErr *net.OpError
	switch {
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil || (errors.As(err, &ne) && ne.Timeout()):
		return &FetchError{Kind: ErrNetwork, Err: fmt.Errorf("request timed out: %w", err)}
	case errors.As(err, &dnsErr), errors.As(err, &opErr):
		return &FetchError{Kind: ErrNetwork, Err: err}
	}
	return &FetchError{Kind: ErrSource, Err: err}
}

// ParseCatalog validates the metadata JSON. A body that is not a JSON object
// of strings is malformed; recognized keys with bad values are listed in
// Invalid, unknown keys in Unrecognized, and neither is ever guessed at.
func ParseCatalog(body []byte) (Catalog, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil || raw == nil {
		return Catalog{}, &FetchError{Kind: ErrMalformed, Err: errors.New("metadata is not a JSON object")}
	}
	if dec.More() {
		return Catalog{}, &FetchError{Kind: ErrMalformed, Err: errors.New("unexpected data after the metadata object")}
	}
	cat := Catalog{Targets: map[string]uint16{}}
	for key, val := range raw {
		var s string
		isString := json.Unmarshal(val, &s) == nil
		if m := targetKeyRe.FindStringSubmatch(key); m != nil {
			target := fmt.Sprintf("%04X", mustHex(m[1]))
			if !isString || !versionHexRe.MatchString(s) {
				cat.Invalid = append(cat.Invalid, key)
				continue
			}
			cat.Targets[target] = uint16(mustHex(s[2:]))
			continue
		}
		if knownNonTarget[key] {
			if isString {
				cat.AppVersion = s
			}
			continue
		}
		cat.Unrecognized = append(cat.Unrecognized, key)
	}
	sort.Strings(cat.Invalid)
	sort.Strings(cat.Unrecognized)
	return cat, nil
}

func mustHex(s string) uint64 {
	v, _ := strconv.ParseUint(s, 16, 32)
	return v
}
