package localdrv

import "os/exec"

type processTree interface {
	configure(cmd *exec.Cmd)

	attach(pid int, lim treeLimits) error

	terminate(pid int) error

	release() error

	limitHits() limitHits
}

type limitHits struct {
	OOMKilled      bool
	MemoryLimitHit bool
	PidsLimitHit   bool
}

type treeLimits struct {
	MemoryBytes  int64
	MaxProcesses int64
}

type Reaping struct {
	Descendants bool

	Detached bool
}
