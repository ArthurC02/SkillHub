//go:build windows

package main

// Docker Desktop on Windows already maps container file ownership back to
// the host, so no numeric UID/GID flags are needed here.
func dockerUserArgs() []string { return nil }
