package update

import (
	"fmt"
	"strconv"
	"strings"
)

// Semver is a parsed semantic version (MAJOR.MINOR.PATCH[-PRERELEASE]).
type Semver struct {
	Major, Minor, Patch int
	Pre                 string
}

// ParseSemver accepts "1.2.3", "v1.2.3" and "v1.2.3-rc.1" (build metadata is ignored).
func ParseSemver(s string) (Semver, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var v Semver
	core := s
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core, v.Pre = s[:i], s[i+1:]
		if v.Pre == "" {
			return Semver{}, fmt.Errorf("invalid version %q", s)
		}
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("invalid version %q", s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || (len(p) > 1 && p[0] == '0') {
			return Semver{}, fmt.Errorf("invalid version %q", s)
		}
		nums[i] = n
	}
	v.Major, v.Minor, v.Patch = nums[0], nums[1], nums[2]
	return v, nil
}

func (v Semver) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Prerelease reports whether the version has a pre-release suffix.
func (v Semver) Prerelease() bool { return v.Pre != "" }

// Compare returns -1, 0 or 1 following semver precedence rules.
func (v Semver) Compare(o Semver) int {
	for _, d := range [3]int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		if d != 0 {
			return sign(d)
		}
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	}
	a, b := strings.Split(v.Pre, "."), strings.Split(o.Pre, ".")
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			continue
		}
		na, ea := strconv.Atoi(a[i])
		nb, eb := strconv.Atoi(b[i])
		switch {
		case ea == nil && eb == nil:
			return sign(na - nb)
		case ea == nil:
			return -1 // numeric identifiers sort before alphanumeric ones
		case eb == nil:
			return 1
		case a[i] < b[i]:
			return -1
		default:
			return 1
		}
	}
	return sign(len(a) - len(b))
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
