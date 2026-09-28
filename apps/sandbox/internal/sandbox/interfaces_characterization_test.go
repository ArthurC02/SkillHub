package sandbox

import (
	"context"
	"time"
)

// preSplitDriverLifecycle and preSplitDriverCapabilities independently spell
// out every method Driver exposed before the split; preSplitDriver below
// checks that set against Driver in both directions.
type preSplitDriverLifecycle interface {
	Start(ctx context.Context, providerRunID string, req RunRequest) error
	Wait(ctx context.Context, providerRunID string) (Outcome, error)
	Stop(ctx context.Context, providerRunID string, grace time.Duration) error
	Remove(ctx context.Context, providerRunID string) error
	ReadTrace(ctx context.Context, providerRunID string, offset int64) (data []byte, more bool, err error)
	ReadArtifacts(ctx context.Context, providerRunID string) ([]byte, error)
	WorkloadDone(ctx context.Context, providerRunID string) (bool, error)
	ReleaseWorkload(ctx context.Context, providerRunID string) error
	Adopt(ctx context.Context) ([]Adopted, error)
}

type preSplitDriverCapabilities interface {
	Healthy(ctx context.Context) bool
	Rootless() bool
	Isolation() IsolationStrength
	DedicatedWorkspacePerRun() bool
	InjectsFromGrant() []string
}

type preSplitDriver interface {
	preSplitDriverLifecycle
	preSplitDriverCapabilities
}

var (
	_ preSplitDriver = Driver(nil)
	_ Driver         = preSplitDriver(nil)
)
