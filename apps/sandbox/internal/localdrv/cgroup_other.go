//go:build !linux && !windows

package localdrv

import "errors"

type cgroup struct{}

func newCgroup(string, treeLimits) (*cgroup, error) {
	return nil, errors.New("resource ceilings need cgroup v2, which this platform does not have")
}

func (c *cgroup) add(int) error { return nil }

func (c *cgroup) remove() error { return nil }

func cgroupEnforcement() ResourceEnforcement { return ResourceEnforcement{} }
