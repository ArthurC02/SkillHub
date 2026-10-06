package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	gvisorReleasePrefix = "release-"
)

var (
	gvisorBaselinePath  = filepath.Join("infra", "nodes", "gvisor-baseline.txt")
	gvisorDaemonPath    = filepath.Join("infra", "deploy", "sandbox", "daemon.json")
	gvisorArchitectures = []string{"x86_64", "aarch64"}
	gvisorSHA512        = regexp.MustCompile(`^[0-9a-f]{128}$`)
	gvisorPinnedArgs    = []string{"--platform=systrap", "--network=sandbox", "--file-access=exclusive"}
)

func gvisorPinProblems(root string) []string {
	var problems []string
	baseline, err := os.ReadFile(filepath.Join(root, gvisorBaselinePath))
	if err != nil {
		problems = append(problems, fmt.Sprintf("%s: %v", gvisorBaselinePath, err))
	} else {
		problems = append(problems, gvisorHashProblems(string(baseline))...)
	}
	daemon, err := os.ReadFile(filepath.Join(root, gvisorDaemonPath))
	if err != nil {
		return append(problems, fmt.Sprintf("%s: %v", gvisorDaemonPath, err))
	}
	return append(problems, gvisorRuntimeArgProblems(daemon)...)
}

func gvisorHashProblems(baselineFile string) []string {
	hashes := map[string]string{}
	release := ""
	for _, line := range strings.Split(baselineFile, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if release == "" {
			release = fields[0]
			continue
		}
		if len(fields) == 3 && fields[0] == "sha512" {
			hashes[fields[1]] = fields[2]
		}
	}
	if !strings.HasPrefix(release, gvisorReleasePrefix) {
		return nil
	}
	var problems []string
	for _, arch := range gvisorArchitectures {
		hash, ok := hashes[arch]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: release %s has no `sha512 %s <hex>` line", gvisorBaselinePath, release, arch))
		case !gvisorSHA512.MatchString(hash):
			problems = append(problems, fmt.Sprintf("%s: sha512 for %s is not 128 lowercase hex digits", gvisorBaselinePath, arch))
		}
	}
	return problems
}

func gvisorRuntimeArgProblems(daemonJSON []byte) []string {
	var daemon struct {
		Runtimes map[string]struct {
			RuntimeArgs []string `json:"runtimeArgs"`
		} `json:"runtimes"`
	}
	if err := json.Unmarshal(daemonJSON, &daemon); err != nil {
		return []string{fmt.Sprintf("%s: %v", gvisorDaemonPath, err)}
	}
	got := daemon.Runtimes["runsc"].RuntimeArgs
	var problems []string
	for _, arg := range got {
		if !contains(gvisorPinnedArgs, arg) {
			problems = append(problems, fmt.Sprintf("%s: runsc runtimeArgs holds %q, outside the pinned set %v", gvisorDaemonPath, arg, gvisorPinnedArgs))
		}
	}
	for _, want := range gvisorPinnedArgs {
		if !contains(got, want) {
			problems = append(problems, fmt.Sprintf("%s: runsc runtimeArgs is missing %q", gvisorDaemonPath, want))
		}
	}
	return problems
}
