package dockerdrv

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func (d *Driver) searchImage(ctx context.Context) (string, sandbox.RuntimeProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	image, err := d.cli.ImageInspect(ctx, d.cfg.Image)
	if err != nil {
		return "", sandbox.RuntimeProfile{}, errors.New("runtime image capability could not be read")
	}
	var profile sandbox.RuntimeProfile
	if image.Config != nil && image.Config.Labels["io.skillhub.runtime.web-search"] == "1" {
		for _, item := range image.Config.Env {
			key, value, _ := strings.Cut(item, "=")
			switch key {
			case "SKILLHUB_RUNTIME":
				profile.Runtime = value
			case "SKILLHUB_RUNTIME_VERSION":
				profile.RuntimeVersion = value
			}
		}
		if profile.Runtime == "" || profile.RuntimeVersion == "" || image.ID == "" {
			return "", sandbox.RuntimeProfile{}, errors.New("runtime image capability is incomplete")
		}
	}
	return image.ID, profile, nil
}

func (d *Driver) WebSearchRuntime(ctx context.Context) (sandbox.RuntimeProfile, error) {
	_, profile, err := d.searchImage(ctx)
	return profile, err
}

func (d *Driver) runImage(ctx context.Context, req sandbox.RunRequest) (string, error) {
	if req.ModelGateway == nil || req.ModelGateway.SearchTool == "" {
		return d.cfg.Image, nil
	}
	resolved, profile, err := d.searchImage(ctx)
	if err != nil {
		return "", err
	}
	if req.ModelGateway.SearchTool != "skillhub-web-search" || profile.Runtime != req.Runtime.Runtime || profile.RuntimeVersion != req.Runtime.RuntimeVersion {
		return "", errors.New("selected runtime image does not support the granted search tool")
	}
	return resolved, nil
}
