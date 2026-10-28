//go:build !unix

package compat

func writable(string) bool { return false }
