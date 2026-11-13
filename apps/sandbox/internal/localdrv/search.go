package localdrv

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const capabilityResponseBytes = 4096

func (d *Driver) WebSearchRuntime(ctx context.Context) (sandbox.RuntimeProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.cfg.NodeBin, d.cfg.RunnerScript, "--capabilities")
	cmd.Env = inheritedHostEnv(nil)
	cmd.WaitDelay = time.Second
	out := &tailWriter{limit: capabilityResponseBytes + 1}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		return sandbox.RuntimeProfile{}, errors.New("local runner capability could not be read")
	}
	var info struct {
		Runtime   string `json:"runtime"`
		Version   string `json:"version"`
		WebSearch *bool  `json:"web_search"`
	}
	if err := json.Unmarshal(out.buf, &info); err != nil || len(out.buf) > capabilityResponseBytes || info.Runtime == "" || info.Version == "" || info.WebSearch == nil {
		return sandbox.RuntimeProfile{}, errors.New("local runner capability is invalid")
	}
	if !*info.WebSearch {
		return sandbox.RuntimeProfile{}, nil
	}
	return sandbox.RuntimeProfile{Runtime: info.Runtime, RuntimeVersion: info.Version}, nil
}
