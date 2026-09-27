package main

import (
	"strings"
	"testing"
)

const testMXCBin = "/opt/mxc/lxc-exec"

func TestSelectDriverPicksTheRequestedBackend(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested string
		runtime   string
		mxcBin    string
		cleanMode bool
		want      string
	}{
		{name: "unset on a normal node keeps docker", want: "docker"},
		{name: "unset on a clean node keeps local", cleanMode: true, want: "local"},
		{name: "docker", requested: "docker", want: "docker"},
		{name: "docker with runsc", requested: "docker", runtime: "runsc", want: "docker"},
		{name: "local on a clean node", requested: "local", cleanMode: true, want: "local"},
		{name: "mxc with its executable", requested: "mxc", mxcBin: testMXCBin, want: "mxc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectDriver(tc.requested, tc.runtime, tc.mxcBin, cleanNode(tc.cleanMode))
			if err != nil {
				t.Fatalf("selectDriver refused a valid combination: %v", err)
			}
			if got != tc.want {
				t.Errorf("selectDriver = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSelectDriverRefusesCombinationsThatCannotHoldTheirPromise(t *testing.T) {
	for _, tc := range []struct {
		name      string
		requested string
		runtime   string
		mxcBin    string
		cleanMode bool
		want      string
	}{
		{
			name: "an unknown driver", requested: "podman",
			want: `SKILLHUB_SANDBOX_DRIVER="podman" is not a driver this node has`,
		},
		{
			name: "a driver name in the wrong case", requested: "MXC", mxcBin: testMXCBin,
			want: `SKILLHUB_SANDBOX_DRIVER="MXC" is not a driver this node has`,
		},
		{
			name: "mxc with runsc", requested: "mxc", runtime: "runsc", mxcBin: testMXCBin,
			want: "SKILLHUB_SANDBOX_DRIVER=mxc cannot run with SKILLHUB_SANDBOX_RUNTIME=runsc",
		},
		{
			name: "mxc on a clean node", requested: "mxc", mxcBin: testMXCBin, cleanMode: true,
			want: "SKILLHUB_SANDBOX_DRIVER=mxc cannot run with SKILLHUB_CLEAN_MODE=1",
		},
		{
			name: "mxc without its executable", requested: "mxc",
			want: "SKILLHUB_SANDBOX_MXC_BIN is required with SKILLHUB_SANDBOX_DRIVER=mxc",
		},
		{
			name: "local without clean mode", requested: "local",
			want: "SKILLHUB_SANDBOX_DRIVER=local requires SKILLHUB_CLEAN_MODE=1",
		},
		{
			name: "docker on a clean node", requested: "docker", cleanMode: true,
			want: "SKILLHUB_SANDBOX_DRIVER=docker cannot run with SKILLHUB_CLEAN_MODE=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectDriver(tc.requested, tc.runtime, tc.mxcBin, cleanNode(tc.cleanMode))
			if err == nil {
				t.Fatalf("selectDriver accepted the combination and chose %q", got)
			}
			if !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("refusal = %q, want it to start with %q", err, tc.want)
			}
			if got != "" {
				t.Errorf("a refused selection still named driver %q", got)
			}
		})
	}
}
