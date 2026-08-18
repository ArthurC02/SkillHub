//go:build windows

package main

// Docker Desktop maps files back to the Windows host; numeric Linux UID/GID
// flags are neither needed nor portable there.
func dockerUserArgs() []string { return nil }
