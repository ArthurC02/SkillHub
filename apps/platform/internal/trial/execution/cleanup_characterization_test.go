package run

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type destroyRecorder struct {
	name      string
	err       error
	destroyed []string
}

func (p *destroyRecorder) Name() string { return p.name }
func (p *destroyRecorder) Capability(context.Context) (ProviderCapability, error) {
	return ProviderCapability{}, nil
}
func (p *destroyRecorder) Start(context.Context, RunRequest) (ProviderRun, error) {
	return ProviderRun{}, nil
}
func (p *destroyRecorder) Observe(context.Context, string) (ProviderRun, error) {
	return ProviderRun{}, nil
}
func (p *destroyRecorder) Cancel(context.Context, string) (ProviderRun, error) {
	return ProviderRun{}, nil
}
func (p *destroyRecorder) Destroy(_ context.Context, id string) error {
	p.destroyed = append(p.destroyed, id)
	return p.err
}
func (p *destroyRecorder) ListActive(context.Context) (ProviderRunList, error) {
	return ProviderRunList{}, nil
}

type revokeRefusing struct{ gatewayStub }

func (revokeRefusing) Revoke(context.Context, string) error { return errors.New("gateway down") }

func attemptInSandbox(provider string) gen.RunAttempt {
	handle := "sbx-1"
	return gen.RunAttempt{
		ID: pgtype.UUID{Bytes: [16]byte{7}, Valid: true}, AttemptNumber: 2,
		Provider: provider, ProviderRunID: &handle,
	}
}

func noHalts() haltState { return haltState{byTarget: map[string]gen.DispatchHalt{}} }

func TestReleasingAnAttemptDestroysItsSandboxAndReportsNothing(t *testing.T) {
	sandbox := &destroyRecorder{name: "alpha"}
	s := &Service{Gateway: gatewaySpending(0), Providers: NewRegistry(sandbox)}

	failures, held := s.releaseAttempt(context.Background(), attemptInSandbox("alpha"), noHalts())

	if held || len(failures) != 0 || !slices.Equal(sandbox.destroyed, []string{"sbx-1"}) {
		t.Errorf("held=%v failures=%v destroyed=%v, want the sandbox destroyed and nothing reported", held, failures, sandbox.destroyed)
	}
}

func TestAnIncidentHaltKeepsTheSandboxAndCountsTheAttemptAsHeld(t *testing.T) {
	sandbox := &destroyRecorder{name: "alpha"}
	s := &Service{Providers: NewRegistry(sandbox)}
	halts := haltState{byTarget: map[string]gen.DispatchHalt{"alpha": {Provider: "alpha", Source: string(HaltSourceIncident)}}}

	failures, held := s.releaseAttempt(context.Background(), attemptInSandbox("alpha"), halts)

	if !held || len(failures) != 0 || len(sandbox.destroyed) != 0 {
		t.Errorf("held=%v failures=%v destroyed=%v, want the scene preserved", held, failures, sandbox.destroyed)
	}
}

func TestAnAttemptThatNeverReachedASandboxIsNeitherHeldNorDestroyed(t *testing.T) {
	sandbox := &destroyRecorder{name: "alpha"}
	s := &Service{Providers: NewRegistry(sandbox)}
	attempt := attemptInSandbox("alpha")
	attempt.ProviderRunID = nil

	failures, held := s.releaseAttempt(context.Background(), attempt, noHalts())

	if held || len(failures) != 0 || len(sandbox.destroyed) != 0 {
		t.Errorf("held=%v failures=%v destroyed=%v, want nothing to do", held, failures, sandbox.destroyed)
	}
}

func TestEveryReleaseFailureIsNamedInOrder(t *testing.T) {
	for _, tc := range []struct {
		name    string
		svc     *Service
		want    []string
		destroy int
	}{
		{
			"the gateway key could not be revoked",
			&Service{Gateway: revokeRefusing{}, Providers: NewRegistry(&destroyRecorder{name: "alpha"})},
			[]string{"model gateway key for attempt 2: gateway down"}, 1,
		},
		{
			"the provider is no longer configured",
			&Service{Providers: NewRegistry(&destroyRecorder{name: "beta"})},
			[]string{"provider alpha is no longer configured"}, 0,
		},
		{
			"the sandbox could not be destroyed",
			&Service{Providers: NewRegistry(&destroyRecorder{name: "alpha", err: errors.New("still running")})},
			[]string{"alpha: still running"}, 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures, held := tc.svc.releaseAttempt(context.Background(), attemptInSandbox("alpha"), noHalts())
			if held || !slices.Equal(failures, tc.want) {
				t.Errorf("held=%v failures=%q, want %q", held, failures, tc.want)
			}
		})
	}
}

func TestAnOrphanCountAtTheThresholdBreachesIt(t *testing.T) {
	for _, tc := range []struct {
		persistent, threshold int64
		want                  bool
	}{{1, 2, false}, {2, 2, true}, {3, 2, true}} {
		if got := (orphanCount{persistent: tc.persistent, threshold: tc.threshold}).breached(); got != tc.want {
			t.Errorf("%d persistent against %d: breached = %v, want %v", tc.persistent, tc.threshold, got, tc.want)
		}
	}
}
