package doccheck

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Phobby/TuiSense/internal/support"
)

const root = "../.."

func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var (
	mdLink   = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	htmlSrc  = regexp.MustCompile(`(?:src|href)="([^"]+)"`)
	heading  = regexp.MustCompile(`(?m)^#{1,6} +(.+)$`)
	fence    = regexp.MustCompile("(?s)```.*?```")
	rawURL   = regexp.MustCompile(`https://raw\.githubusercontent\.com/([^/\s]+)/([^/\s]+)/main/([^\s"')]+)`)
	repoLink = regexp.MustCompile(`github\.com/([A-Za-z0-9-]+)/([A-Za-z0-9_.-]+?)(?:\.git)?(?:[/)\s"#]|$)`)
)

// slug mirrors GitHub's heading anchors.
func slug(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	var b strings.Builder
	for _, r := range h {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func markdownFiles(t *testing.T) []string {
	files := []string{"README.md"}
	for _, dir := range []string{"docs", "docs/media"} {
		matches, _ := filepath.Glob(filepath.Join(root, dir, "*.md"))
		for _, m := range matches {
			rel, _ := filepath.Rel(root, m)
			files = append(files, filepath.ToSlash(rel))
		}
	}
	return files
}

func TestLocalLinksAndAnchorsResolve(t *testing.T) {
	for _, file := range markdownFiles(t) {
		text := fence.ReplaceAllString(read(t, file), "")
		anchors := map[string]bool{}
		for _, m := range heading.FindAllStringSubmatch(text, -1) {
			anchors[slug(m[1])] = true
		}
		var targets []string
		for _, m := range mdLink.FindAllStringSubmatch(text, -1) {
			targets = append(targets, m[1])
		}
		for _, m := range htmlSrc.FindAllStringSubmatch(text, -1) {
			targets = append(targets, m[1])
		}
		for _, target := range targets {
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			path, frag, _ := strings.Cut(target, "#")
			if path == "" {
				if !anchors[frag] {
					t.Errorf("%s: link to missing heading #%s", file, frag)
				}
				continue
			}
			full := filepath.Join(root, filepath.Dir(file), path)
			if _, err := os.Stat(full); err != nil {
				t.Errorf("%s: link target %q does not exist", file, target)
			}
		}
	}
}

func TestReadmeMediaIsRealAndReasonablySized(t *testing.T) {
	readme := read(t, "README.md")
	refs := regexp.MustCompile(`docs/media/[\w.-]+\.(?:gif|png|mp4)`).FindAllString(readme, -1)
	if len(refs) < 8 {
		t.Fatalf("README references %d media files; every feature section should have one", len(refs))
	}
	for _, ref := range refs {
		b, err := os.ReadFile(filepath.Join(root, ref))
		if err != nil {
			t.Errorf("README shows %s, but it does not exist (use a labelled placeholder instead)", ref)
			continue
		}
		if strings.HasSuffix(ref, ".gif") && !bytes.HasPrefix(b, []byte("GIF89a")) {
			t.Errorf("%s is not a GIF", ref)
		}
		if len(b) > 2<<20 {
			t.Errorf("%s is %d KB; keep README media under 2 MB", ref, len(b)>>10)
		}
	}
	start := strings.Index(readme, "<!-- TUISENSE_PROMO_VIDEO_START -->")
	end := strings.Index(readme, "<!-- TUISENSE_PROMO_VIDEO_END -->")
	if start < 0 || end < start {
		t.Fatal("the promotional video placeholder markers are missing or out of order")
	}
	if !strings.Contains(readme[start:end], "## See TuiSense in Action") {
		t.Error("the promotional video section lost its heading")
	}
	if !strings.Contains(readme, "MOCK") {
		t.Error("README must say its demonstrations use simulated (MOCK) devices")
	}
}

func TestInstallCommandsReferenceThisRepository(t *testing.T) {
	gomod := read(t, "go.mod")
	m := regexp.MustCompile(`(?m)^module github\.com/([^/\s]+)/([^/\s]+)$`).FindStringSubmatch(gomod)
	if m == nil {
		t.Fatal("go.mod does not declare a github.com module path")
	}
	owner, repo := m[1], m[2]
	readme := read(t, "README.md")
	urls := rawURL.FindAllStringSubmatch(readme, -1)
	if len(urls) == 0 {
		t.Fatal("README has no installer commands")
	}
	for _, u := range urls {
		if u[1] != owner || u[2] != repo {
			t.Errorf("installer URL %s is not this repository (%s/%s)", u[0], owner, repo)
		}
		if _, err := os.Stat(filepath.Join(root, u[3])); err != nil {
			t.Errorf("installer URL %s points at a file that does not exist", u[0])
		}
	}
	for _, l := range repoLink.FindAllStringSubmatch(readme, -1) {
		if l[1] == "thebanri" { // Limoni, acknowledged dependency
			continue
		}
		if l[1] != owner || l[2] != repo {
			t.Errorf("README links to another repository: github.com/%s/%s", l[1], l[2])
		}
	}
	// Installers must name the same repository they are served from.
	for _, script := range []string{"scripts/install.sh", "scripts/install.ps1"} {
		if !strings.Contains(read(t, script), owner+"/"+repo) {
			t.Errorf("%s does not download from %s/%s", script, owner, repo)
		}
	}
}

// supportRows parses the README "Supported Controllers" table.
func supportRows(t *testing.T) [][]string {
	readme := read(t, "README.md")
	i := strings.Index(readme, "## Supported Controllers")
	if i < 0 {
		t.Fatal("README has no Supported Controllers section")
	}
	var rows [][]string
	for _, line := range strings.Split(readme[i:], "\n") {
		if !strings.HasPrefix(line, "| ") {
			if len(rows) > 0 {
				break
			}
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if cells[0] == "Controller" || strings.HasPrefix(cells[0], "---") {
			continue
		}
		rows = append(rows, cells)
	}
	return rows
}

func TestReadmeSupportClaimsMatchDrivers(t *testing.T) {
	rows := supportRows(t)
	matrix := support.Matrix()
	if len(rows) != len(matrix) {
		t.Fatalf("README lists %d controllers, the drivers declare %d", len(rows), len(matrix))
	}
	implemented := func(cell string) bool { return strings.HasPrefix(cell, "Implemented") }
	for i, cells := range rows {
		if len(cells) != 4 {
			t.Fatalf("row %q: want 4 columns", cells)
		}
		name, linux, windows, macos := cells[0], strings.ToLower(cells[1]), cells[2], cells[3]
		r := matrix[i]
		for _, w := range strings.Fields(name) {
			if w != "Sony" && !strings.Contains(r.Model, w) {
				t.Fatalf("README row %d (%s) does not match driver %s; keep the table in matrix order", i, name, r.Model)
			}
		}
		if got, want := strings.HasPrefix(linux, "✅"), strings.Contains(r.Platforms, "Linux"); got != want {
			t.Errorf("%s: README Linux support %v, drivers %v", name, got, want)
		}
		if got, want := strings.Contains(windows, "🧪"), strings.Contains(r.Platforms, "Windows"); got != want {
			t.Errorf("%s: README Windows support %v, drivers %v", name, got, want)
		}
		if strings.Contains(r.Platforms, "Windows") && strings.Contains(r.Platforms, "experimental") && !strings.Contains(windows, "🧪") {
			t.Errorf("%s: Windows support is experimental and must be marked so", name)
		}
		if macos != "—" || strings.Contains(r.Platforms, "macOS") {
			t.Errorf("%s: README and drivers disagree about macOS (%q vs %q)", name, macos, r.Platforms)
		}
		claims := map[string]string{"battery": r.Battery, "lighting": r.Lighting, "adaptive triggers": r.Triggers, "rumble": r.Vibration, "haptics": r.Vibration}
		for word, cell := range claims {
			if strings.Contains(linux, word) && !implemented(cell) {
				t.Errorf("%s: README claims %s on Linux, drivers say %q", name, word, cell)
			}
		}
		if implemented(r.Battery) != strings.Contains(linux, "battery") {
			t.Errorf("%s: README battery claim disagrees with drivers (%q)", name, r.Battery)
		}
		if strings.Contains(r.Verification, "erified on hardware") && !strings.Contains(r.Verification, "not verified") {
			t.Errorf("%s: drivers now claim hardware verification; update the README note", name)
		}
	}
}
