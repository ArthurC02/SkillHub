//go:build linux

package dockerdrv

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

const (
	cgroupMount  = "/sys/fs/cgroup"
	pidsPollTick = 50 * time.Millisecond
)

func watchPids(pid int, containerID, id string, log *slog.Logger) *pidsWatch {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cgroup")
	if err != nil {
		warn(log, "cannot locate the sandbox's cgroup; a pid limit hit cannot be reported", "provider_run_id", id, "err", err)
		return nil
	}
	rel, ok := cgroupV2Path(string(raw), containerID)
	if !ok {
		warn(log, "sandbox has no cgroup v2 directory of its own; a pid limit hit cannot be reported", "provider_run_id", id)
		return nil
	}
	path := filepath.Join(cgroupMount, rel, "pids.events")

	w := &pidsWatch{stop: make(chan struct{}), log: log, id: id}
	notify, err := notifyOnModify(path)
	if err != nil {
		warn(log, "inotify unavailable for pids.events; polling only", "provider_run_id", id, "err", err)
	} else {
		w.release = func() { _ = notify.Close() }
	}

	w.check(path)
	w.wg.Add(1)
	go w.poll(path)
	if notify != nil {
		w.wg.Add(1)
		go w.follow(path, notify)
	}
	return w
}

func notifyOnModify(path string) (*os.File, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	if _, err := unix.InotifyAddWatch(fd, path, unix.IN_MODIFY); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "pids-events-inotify"), nil
}

func (w *pidsWatch) follow(path string, notify *os.File) {
	defer w.wg.Done()
	buf := make([]byte, 4096)
	for {
		if _, err := notify.Read(buf); err != nil {
			return
		}
		w.check(path)
	}
}

func (w *pidsWatch) poll(path string) {
	defer w.wg.Done()
	tick := time.NewTicker(pidsPollTick)
	defer tick.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-tick.C:
			w.check(path)
		}
	}
}

func (w *pidsWatch) check(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	w.readable.Store(true)
	if n, ok := pidsLimitEvents(string(raw)); ok && n > 0 {
		w.hit.Store(true)
	}
}
