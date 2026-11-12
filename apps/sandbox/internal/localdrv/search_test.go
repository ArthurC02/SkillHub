package localdrv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func capabilityRunner(t *testing.T, script string) *Driver {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runner.mjs")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{NodeBin: requireNode(t), RunnerScript: path, BaseDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

const supportedRunner = `
if (!process.argv.includes('--capabilities')) process.exit(0);
if (process.env.ANTHROPIC_AUTH_TOKEN || process.env.SKILLHUB_SEARCH_TOOL || process.env.NODE_OPTIONS) process.exit(12);
console.log(JSON.stringify({runtime: 'claude_agent_sdk', version: '0.3.233', web_search: true}));
`

func TestLocalSearchCapabilityReadsTheRunnerWithoutGrantOrModelAccess(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "must-not-inherit")
	t.Setenv("SKILLHUB_SEARCH_TOOL", "must-not-inherit")
	t.Setenv("NODE_OPTIONS", "--skillhub-invalid-option")
	d := capabilityRunner(t, supportedRunner)
	profile, err := d.WebSearchRuntime(context.Background())
	if err != nil || profile.Runtime != "claude_agent_sdk" || profile.RuntimeVersion != "0.3.233" {
		t.Fatalf("actual runner capability = %+v, %v", profile, err)
	}
}

func TestLocalSearchCapabilityDistinguishesUnsupportedFromUnreadable(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		bad          bool
	}{
		{"old runner with explicit refusal", `console.log('{"runtime":"claude_agent_sdk","version":"0.3.232","web_search":false}')`, false},
		{"old CLI", `process.exit(2)`, true},
		{"invalid output", `console.log('invalid')`, true},
		{"missing version", `console.log('{"runtime":"claude_agent_sdk","web_search":true}')`, true},
		{"missing flag", `console.log('{"runtime":"claude_agent_sdk","version":"0.3.233"}')`, true},
		{"null flag", `console.log('{"runtime":"claude_agent_sdk","version":"0.3.233","web_search":null}')`, true},
		{"wrong flag type", `console.log('{"runtime":"claude_agent_sdk","version":"0.3.233","web_search":"true"}')`, true},
		{"oversized response", `process.stdout.write(' '.repeat(4097)+'{"runtime":"claude_agent_sdk","version":"0.3.233","web_search":true}')`, true},
		{"multiple objects", `console.log('{}{}')`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := capabilityRunner(t, tc.script)
			profile, err := d.WebSearchRuntime(context.Background())
			if (err != nil) != tc.bad || profile.Runtime != "" {
				t.Fatalf("unsupported/unreadable runner = %+v, %v", profile, err)
			}
		})
	}
}

func TestLocalSearchCapabilityStopsWhenItsCallerCancels(t *testing.T) {
	d := capabilityRunner(t, `setInterval(() => {}, 1000)`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if profile, err := d.WebSearchRuntime(ctx); err == nil || profile.Runtime != "" {
		t.Fatalf("cancelled capability read = %+v, %v", profile, err)
	}
}

func TestLocalSearchCapabilityResponseBoundary(t *testing.T) {
	for _, size := range []int{4096, 4097} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			script := fmt.Sprintf(`const text=JSON.stringify({runtime:'claude_agent_sdk',version:'0.3.233',web_search:true}); process.stdout.write(text.padEnd(%d,' '));`, size)
			d := capabilityRunner(t, script)
			profile, err := d.WebSearchRuntime(context.Background())
			if size == 4096 && (err != nil || profile.Runtime != "claude_agent_sdk") {
				t.Fatalf("maximum-size capability was refused: %+v, %v", profile, err)
			}
			if size == 4097 && (err == nil || profile.Runtime != "") {
				t.Fatalf("oversized capability was accepted: %+v, %v", profile, err)
			}
		})
	}
}

func TestLocalSearchStartChecksTheCurrentRunnerBeforePreparingInputs(t *testing.T) {
	for _, tc := range []struct {
		name, script, version, tool string
		want                        bool
	}{
		{"supported", supportedRunner, "0.3.233", "skillhub-web-search", true},
		{"replaced runner", `console.log('{"runtime":"claude_agent_sdk","version":"0.3.233","web_search":false}')`, "0.3.233", "skillhub-web-search", false},
		{"wrong version", supportedRunner, "0.3.232", "skillhub-web-search", false},
		{"wrong alias", supportedRunner, "0.3.233", "other-search", false},
		{"ordinary run does not need search CLI", `process.exit(0)`, "0.3.233", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := capabilityRunner(t, supportedRunner)
			if _, err := d.WebSearchRuntime(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(d.cfg.RunnerScript, []byte(tc.script), 0o600); err != nil {
				t.Fatal(err)
			}
			req := minimalRequest("search-capability")
			req.Runtime.RuntimeVersion = tc.version
			if tc.tool != "" {
				req.ModelGateway = &sandbox.ModelGatewayGrant{SearchTool: tc.tool}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := d.Start(ctx, "search-capability", req)
			if (err == nil) != tc.want {
				t.Fatalf("Start = %v, want success %v", err, tc.want)
			}
			if !tc.want {
				assertNoSearchWorkloadInputs(t, d)
				return
			}
			if _, err := d.Wait(ctx, "search-capability"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func assertNoSearchWorkloadInputs(t *testing.T, d *Driver) {
	t.Helper()
	work, _ := d.paths("search-capability")
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Fatalf("unsupported search prepared workload inputs: %v", err)
	}
}
