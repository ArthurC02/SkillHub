//go:build !windows

package main

import (
	"fmt"
	"os"
)

func dockerUserArgs() []string {
	return []string{"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())}
}
