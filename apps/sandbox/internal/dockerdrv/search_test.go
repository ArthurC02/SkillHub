package dockerdrv

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/moby/moby/client"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

type searchImageEngine struct {
	engine
	image       client.ImageInspectResult
	err         error
	inspected   string
	createdWith string
}

func (e *searchImageEngine) ImageInspect(ctx context.Context, name string, _ ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	if _, ok := ctx.Deadline(); !ok {
		return client.ImageInspectResult{}, errors.New("image inspection must be bounded")
	}
	e.inspected = name
	return e.image, e.err
}

func (e *searchImageEngine) ContainerCreate(_ context.Context, options client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	e.createdWith = options.Config.Image
	return client.ContainerCreateResult{}, errors.New("stop after recording the selected image")
}

func supportingImage(t *testing.T) client.ImageInspectResult {
	t.Helper()
	var result client.ImageInspectResult
	if err := json.Unmarshal([]byte(`{"Id":"sha256:actual-image","Config":{"Labels":{"io.skillhub.runtime.web-search":"1"},"Env":["SKILLHUB_RUNTIME=claude_agent_sdk","SKILLHUB_RUNTIME_VERSION=0.3.233"]}}`), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDockerSearchCapabilityReadsTheSelectedImageIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*searchImageEngine)
		want   sandbox.RuntimeProfile
		bad    bool
	}{
		{"supported", func(*searchImageEngine) {}, sandbox.RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "0.3.233"}, false},
		{"old image", func(e *searchImageEngine) { e.image.Config.Labels = nil }, sandbox.RuntimeProfile{}, false},
		{"different label", func(e *searchImageEngine) { e.image.Config.Labels["io.skillhub.runtime.web-search"] = "true" }, sandbox.RuntimeProfile{}, false},
		{"no config", func(e *searchImageEngine) { e.image.Config = nil }, sandbox.RuntimeProfile{}, false},
		{"unreadable image", func(e *searchImageEngine) { e.err = errors.New("unavailable") }, sandbox.RuntimeProfile{}, true},
		{"missing version", func(e *searchImageEngine) { e.image.Config.Env = e.image.Config.Env[:1] }, sandbox.RuntimeProfile{}, true},
		{"missing runtime", func(e *searchImageEngine) { e.image.Config.Env = e.image.Config.Env[1:] }, sandbox.RuntimeProfile{}, true},
		{"missing image id", func(e *searchImageEngine) { e.image.ID = "" }, sandbox.RuntimeProfile{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &searchImageEngine{image: supportingImage(t)}
			tc.change(engine)
			d := &Driver{cli: engine, cfg: Config{Image: "configured-image:tag"}}
			got, err := d.WebSearchRuntime(context.Background())
			if (err != nil) != tc.bad || got != tc.want || engine.inspected != "configured-image:tag" {
				t.Fatalf("capability = %+v, %v, inspected=%q", got, err, engine.inspected)
			}
		})
	}
}

func TestDockerSearchStartPinsTheVerifiedImageAndRefusesStaleEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, version, tool string
		removeLabel         bool
		wantImage           string
	}{
		{"supported", "0.3.233", "skillhub-web-search", false, "sha256:actual-image"},
		{"old image after capability read", "0.3.233", "skillhub-web-search", true, ""},
		{"wrong version", "0.3.232", "skillhub-web-search", false, ""},
		{"wrong alias", "0.3.233", "other-search", false, ""},
		{"ordinary run", "0.3.233", "", true, "configured-image:tag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := &searchImageEngine{image: supportingImage(t)}
			d := &Driver{cli: engine, cfg: Config{Image: "configured-image:tag"}}
			if _, err := d.WebSearchRuntime(context.Background()); err != nil {
				t.Fatal(err)
			}
			if tc.removeLabel {
				engine.image.Config.Labels = nil
			}
			req := sandbox.RunRequest{Runtime: sandbox.RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: tc.version}}
			if tc.tool != "" {
				req.ModelGateway = &sandbox.ModelGatewayGrant{SearchTool: tc.tool}
			}
			err := d.Start(context.Background(), "probe", req)
			if err == nil || engine.createdWith != tc.wantImage {
				t.Fatalf("Start created image %q, want %q, err=%v", engine.createdWith, tc.wantImage, err)
			}
		})
	}
}
