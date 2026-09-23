//go:build linux

package localdrv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const cgroupMount = "/sys/fs/cgroup"

var errNoCgroup = errors.New("no writable cgroup v2 subtree with the memory and pids controllers")

var wanted = []string{"memory", "pids"}

type cgroup struct {
	dir string
}

// cgroup v2 refuses to hand controllers to the children of a cgroup that still
// holds processes of its own, so the supervisor moves into a leaf of its own
// cgroup before it can place any workload beside it.
var runParent = sync.OnceValues(func() (string, error) {
	own, err := ownCgroupDir()
	if err != nil {
		return "", err
	}
	granted := grantedControllers(own)
	if len(granted) == 0 {
		return "", errNoCgroup
	}
	subtree := filepath.Join(own, "cgroup.subtree_control")
	if subtreeHas(subtree, granted) {
		return own, nil
	}

	leaf := filepath.Join(own, "supervisor")
	if err := os.Mkdir(leaf, 0o755); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("carve a leaf for the supervisor: %w", err)
	}
	self := strconv.Itoa(os.Getpid())
	if err := writeFile(filepath.Join(leaf, "cgroup.procs"), self); err != nil {
		return "", fmt.Errorf("move the supervisor into its leaf: %w", err)
	}
	if err := writeFile(subtree, "+"+strings.Join(granted, " +")); err != nil {
		_ = writeFile(filepath.Join(own, "cgroup.procs"), self)
		_ = os.Remove(leaf)
		return "", fmt.Errorf("delegate %s to the workloads: %w", strings.Join(granted, ", "), err)
	}
	return own, nil
})

func subtreeHas(path string, want []string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	have := strings.Fields(string(raw))
	for _, c := range want {
		if !slices.Contains(have, c) {
			return false
		}
	}
	return true
}

func ownCgroupDir() (string, error) {
	raw, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		rel, ok := strings.CutPrefix(line, "0::")
		if !ok {
			continue
		}
		dir := filepath.Join(cgroupMount, rel)
		if _, err := os.Stat(dir); err != nil {
			return "", fmt.Errorf("own cgroup %s is not mounted here: %w", rel, err)
		}
		return dir, nil
	}
	return "", errNoCgroup
}

func grantedControllers(dir string) []string {
	raw, err := os.ReadFile(filepath.Join(dir, "cgroup.controllers"))
	if err != nil {
		return nil
	}
	have := strings.Fields(string(raw))
	var granted []string
	for _, c := range wanted {
		if slices.Contains(have, c) {
			granted = append(granted, c)
		}
	}
	return granted
}

func writeFile(path, value string) error {
	return os.WriteFile(path, []byte(value), 0o644)
}

func newCgroup(name string, lim treeLimits) (*cgroup, error) {
	parent, err := runParent()
	if err != nil {
		return nil, err
	}
	c := &cgroup{dir: filepath.Join(parent, name)}
	if err := os.Mkdir(c.dir, 0o755); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("create cgroup %s: %w", name, err)
	}
	if lim.MemoryBytes > 0 {
		if err := writeFile(filepath.Join(c.dir, "memory.max"), strconv.FormatInt(lim.MemoryBytes, 10)); err != nil {
			_ = c.remove()
			return nil, fmt.Errorf("set memory ceiling: %w", err)
		}
		// Without this the workload swaps past the ceiling instead of hitting it.
		_ = writeFile(filepath.Join(c.dir, "memory.swap.max"), "0")
	}
	if lim.MaxProcesses > 0 {
		if err := writeFile(filepath.Join(c.dir, "pids.max"), strconv.FormatInt(lim.MaxProcesses, 10)); err != nil {
			_ = c.remove()
			return nil, fmt.Errorf("set process ceiling: %w", err)
		}
	}
	return c, nil
}

func (c *cgroup) add(pid int) error {
	return writeFile(filepath.Join(c.dir, "cgroup.procs"), strconv.Itoa(pid))
}

// rmdir only succeeds on an empty cgroup, and a killed process stays a member
// until the kernel reaps it.
func (c *cgroup) remove() error {
	_ = writeFile(filepath.Join(c.dir, "cgroup.kill"), "1")
	var err error
	for range 50 {
		if err = os.Remove(c.dir); err == nil || os.IsNotExist(err) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return err
}

func cgroupEnforcement() ResourceEnforcement {
	parent, err := runParent()
	if err != nil || unix.Access(parent, unix.W_OK) != nil {
		return ResourceEnforcement{}
	}
	subtree := filepath.Join(parent, "cgroup.subtree_control")
	return ResourceEnforcement{
		Memory:    subtreeHas(subtree, []string{"memory"}),
		Processes: subtreeHas(subtree, []string{"pids"}),
	}
}
