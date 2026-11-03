//go:build !windows

package doctor

import "runtime"

func osVersion() string { return runtime.GOOS }
