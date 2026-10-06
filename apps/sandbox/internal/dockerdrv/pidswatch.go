package dockerdrv

import (
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type pidsWatch struct {
	hit      atomic.Bool
	readable atomic.Bool
	stop     chan struct{}
	wg       sync.WaitGroup
	once     sync.Once
	release  func()
	log      *slog.Logger
	id       string
}

func (w *pidsWatch) Stop() bool {
	if w == nil {
		return false
	}
	w.once.Do(func() {
		close(w.stop)
		if w.release != nil {
			w.release()
		}
		w.wg.Wait()
		if !w.readable.Load() {
			warn(w.log, "pids.events was never readable; a pid limit hit cannot be reported", "provider_run_id", w.id)
		}
	})
	return w.hit.Load()
}

func warn(log *slog.Logger, msg string, args ...any) {
	if log != nil {
		log.Warn(msg, args...)
	}
}

func pidsLimitEvents(raw string) (int64, bool) {
	for _, line := range strings.Split(raw, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found || key != "max" {
			continue
		}
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 0 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

func cgroupV2Path(procCgroup, containerID string) (string, bool) {
	for _, line := range strings.Split(procCgroup, "\n") {
		if rel, ok := strings.CutPrefix(line, "0::"); ok && strings.HasPrefix(rel, "/") && strings.Contains(rel, containerID) {
			return rel, true
		}
	}
	return "", false
}
