package sony

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// liveSample is the shape of Sony's metadata as observed in October 2026.
const liveSample = `{
    "ApplicationLatestVersion": "2.2.1.2",
    "FwUpdate0004LatestVersion": "0x0630",
    "FwUpdate000BLatestVersion": "0x0630",
    "FwUpdate000ELatestVersion": "0x0641",
    "FwUpdate0044LatestVersion": "0x0217"
}`

func kindOf(err error) (ErrorKind, bool) {
	var fe *FetchError
	if errors.As(err, &fe) {
		return fe.Kind, true
	}
	return 0, false
}

func TestParseCatalogValid(t *testing.T) {
	cat, err := ParseCatalog([]byte(liveSample))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]uint16{"0004": 0x0630, "000B": 0x0630, "000E": 0x0641, "0044": 0x0217}
	for k, v := range want {
		if cat.Targets[k] != v {
			t.Errorf("target %s = %#x, want %#x", k, cat.Targets[k], v)
		}
	}
	if cat.AppVersion != "2.2.1.2" || len(cat.Invalid) != 0 || len(cat.Unrecognized) != 0 {
		t.Fatalf("unexpected catalog %+v", cat)
	}
}

func TestParseCatalogMalformed(t *testing.T) {
	for _, body := range []string{``, `not json`, `[1,2,3]`, `null`, `{"a":1} {"b":2}`, `<html>maintenance</html>`} {
		_, err := ParseCatalog([]byte(body))
		if k, ok := kindOf(err); !ok || k != ErrMalformed {
			t.Errorf("%q: want ErrMalformed, got %v", body, err)
		}
	}
}

func TestParseCatalogUnexpectedChanges(t *testing.T) {
	cat, err := ParseCatalog([]byte(`{
		"FwUpdate0004LatestVersion": 1584,
		"FwUpdate000BLatestVersion": "6.30",
		"FwUpdate000ELatestVersion": "0x123456",
		"FwUpdate0044LatestVersion": "0x0217",
		"Channel": "stable"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Targets) != 1 || cat.Targets["0044"] != 0x0217 {
		t.Fatalf("only well-formed values may be used, got %+v", cat.Targets)
	}
	if strings.Join(cat.Invalid, ",") != "FwUpdate0004LatestVersion,FwUpdate000BLatestVersion,FwUpdate000ELatestVersion" {
		t.Fatalf("invalid entries %v", cat.Invalid)
	}
	if len(cat.Unrecognized) != 1 || cat.Unrecognized[0] != "Channel" {
		t.Fatalf("unrecognized %v", cat.Unrecognized)
	}
}

// tlsServer serves handler over HTTPS and records requested paths.
func tlsServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *HTTPFetcher, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	f := &HTTPFetcher{URL: srv.URL + "/fwupdater/info.json", Client: srv.Client(), UserAgent: "TuiSense-test"}
	return srv, f, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), paths...)
	}
}

func TestHTTPFetcherSuccessOnlyFetchesMetadata(t *testing.T) {
	_, f, paths := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "TuiSense-test" {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(liveSample))
	})
	cat, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cat.Targets["0044"] != 0x0217 {
		t.Fatalf("catalog %+v", cat)
	}
	// A second fetch within the cache window must not hit the server again.
	if _, err := f.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := paths()
	if len(got) != 1 || got[0] != "GET /fwupdater/info.json" {
		t.Fatalf("only the metadata file may be requested, once: %v", got)
	}
}

func TestHTTPFetcherServerErrors(t *testing.T) {
	_, f, _ := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	})
	if k, ok := kindOf(func() error { _, err := f.Fetch(context.Background()); return err }()); !ok || k != ErrSource {
		t.Fatal("HTTP 404 must be a source error")
	}

	_, big, _ := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"x":"` + strings.Repeat("a", maxCatalogBytes) + `"}`))
	})
	if k, ok := kindOf(func() error { _, err := big.Fetch(context.Background()); return err }()); !ok || k != ErrMalformed {
		t.Fatal("an oversized response must be rejected as malformed")
	}

	_, bad, _ := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>Service unavailable</html>`))
	})
	if k, ok := kindOf(func() error { _, err := bad.Fetch(context.Background()); return err }()); !ok || k != ErrMalformed {
		t.Fatal("an HTML page must be rejected as malformed")
	}
}

func TestHTTPFetcherTimeout(t *testing.T) {
	release := make(chan struct{})
	_, f, _ := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := f.Fetch(ctx)
	if k, ok := kindOf(err); !ok || k != ErrNetwork {
		t.Fatalf("timeout must be a network error, got %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("the timeout was not honored")
	}
}

func TestHTTPFetcherOffline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens here any more
	f := &HTTPFetcher{URL: "https://" + addr + "/fwupdater/info.json"}
	_, err = f.Fetch(context.Background())
	if k, ok := kindOf(err); !ok || k != ErrNetwork {
		t.Fatalf("connection refused must be a network error, got %v", err)
	}
}

func TestHTTPFetcherRefusesInsecureURLs(t *testing.T) {
	f := &HTTPFetcher{URL: "http://fwupdater.dl.playstation.net/fwupdater/info.json"}
	if k, ok := kindOf(func() error { _, err := f.Fetch(context.Background()); return err }()); !ok || k != ErrSource {
		t.Fatal("plain HTTP must be refused")
	}

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(liveSample)) }))
	defer plain.Close()
	_, redir, _ := tlsServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/info.json", http.StatusFound)
	})
	if _, err := redir.Fetch(context.Background()); err == nil {
		t.Fatal("a redirect to plain HTTP must be refused")
	}
}
