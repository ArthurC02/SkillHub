package dockerdrv_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/moby/moby/client"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/dockerdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

type faultyEngine struct {
	dockerdrv.Engine
	noAddress bool
	noReady   bool
	goneFirst bool
}

func (f faultyEngine) ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	if f.noAddress && !strings.HasPrefix(containerID, "skillhub-run-") {
		return client.ContainerInspectResult{}, nil
	}
	return f.Engine.ContainerInspect(ctx, containerID, options)
}

func (f faultyEngine) ExecCreate(ctx context.Context, containerID string, options client.ExecCreateOptions) (client.ExecCreateResult, error) {
	if f.noReady && slices.Contains(options.Cmd, "of="+dockerdrv.ReadyPath) {
		if f.goneFirst {
			if _, err := f.ContainerStop(ctx, containerID, client.ContainerStopOptions{}); err != nil {
				return client.ExecCreateResult{}, err
			}
		}
		return client.ExecCreateResult{}, errors.New("exec refused")
	}
	return f.Engine.ExecCreate(ctx, containerID, options)
}

func startFaulty(t *testing.T, network string, fault faultyEngine) error {
	t.Helper()
	newDriver(t)
	d, err := dockerdrv.New(dockerdrv.Config{
		Image:       testImage(),
		Network:     network,
		UID:         65532,
		GID:         65532,
		AllowDevCmd: true,
		Runtime:     testRuntime(),
		ExtraLabels: map[string]string{testLabel: "1"},
		Log:         slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
	})
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	dockerdrv.WrapEngine(d, func(e dockerdrv.Engine) dockerdrv.Engine {
		fault.Engine = e
		return fault
	})
	id := handle(t)
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })
	req := testRequest("sleep 30")
	if network != "none" {
		req.Egress.Allow = []sandbox.EgressAllowEntry{{Purpose: "model_gateway", URL: "http://10.9.9.9:4000"}}
	}
	return d.Start(context.Background(), id, req)
}

func TestANetworkedRunWhoseAddressCannotBeReadDoesNotStart(t *testing.T) {
	err := startFaulty(t, "bridge", faultyEngine{noAddress: true})
	if !errors.Is(err, dockerdrv.ErrAddressUnreadable) {
		t.Fatalf("Start = %v, want ErrAddressUnreadable: a run whose egress cannot be attributed must not run", err)
	}
}

func TestARunTheDriverCannotTellItsInputsAreReadyDoesNotStart(t *testing.T) {
	err := startFaulty(t, "none", faultyEngine{noReady: true})
	if err == nil || !strings.Contains(err.Error(), "exec refused") {
		t.Fatalf("Start = %v, want the ready signal's failure: otherwise the workload waits for inputs that never arrive", err)
	}
}

func TestASandboxThatEndedBeforeItsReadySignalIsNotAProvisionFailure(t *testing.T) {
	if err := startFaulty(t, "none", faultyEngine{noReady: true, goneFirst: true}); err != nil {
		t.Fatalf("Start = %v, want nil: the run has already ended and its own outcome says why", err)
	}
}
