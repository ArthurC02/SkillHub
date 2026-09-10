package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	runtimeDockerfile = "infra/images/runtime-agent-sdk/Dockerfile"
	runtimeUpgrades   = "infra/images/runtime-agent-sdk/UPGRADES.md"
)

// Matches "ARG IMAGE_VERSION=<value>", the value optionally quoted.
var imageVersionArg = regexp.MustCompile(`(?m)^ARG\s+IMAGE_VERSION\s*=\s*"?([^"\s]+)"?\s*$`)

func imageVersionProblems(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runtimeDockerfile)))
	if err != nil {
		return []string{fmt.Sprintf("image-version: %v", err)}
	}
	matches := imageVersionArg.FindAllStringSubmatch(string(data), -1)
	switch len(matches) {
	case 1:
	case 0:
		return []string{fmt.Sprintf(
			"image-version: %s no longer declares `ARG IMAGE_VERSION=`; runtime-image.yml's I-05 gate and "+
				"ADR-023 §4's upgrade record are both keyed on it, so this check has lost its subject",
			runtimeDockerfile)}
	default:
		return []string{fmt.Sprintf(
			"image-version: %s declares `ARG IMAGE_VERSION=` %d times; the image has one version",
			runtimeDockerfile, len(matches))}
	}
	version := matches[0][1]

	upgrades, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runtimeUpgrades)))
	if err != nil {
		return []string{fmt.Sprintf("image-version: %v", err)}
	}
	for _, line := range strings.Split(string(upgrades), "\n") {
		if strings.HasPrefix(line, "#") && strings.Contains(line, version) {
			return nil
		}
	}
	return []string{fmt.Sprintf(
		"image-version: %s pins IMAGE_VERSION=%s and no heading in %s mentions it. ADR-023 §4 requires the "+
			"four measurements to be re-run and written down on every bump, and this image's failure mode "+
			"is silent — an SDK that stops emitting tool events yields a thinner trace, not a crash. Add "+
			"the section, or say in it which measurements were carried over and why",
		runtimeDockerfile, version, runtimeUpgrades)}
}
