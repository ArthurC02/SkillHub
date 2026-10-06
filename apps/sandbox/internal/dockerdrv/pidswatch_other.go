//go:build !linux

package dockerdrv

import "log/slog"

func watchPids(int, string, string, *slog.Logger) *pidsWatch { return nil }
