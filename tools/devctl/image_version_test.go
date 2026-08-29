package main

import (
	"strings"
	"testing"
)

// Pointed at the tree. RED until UPGRADES.md carries a section for whatever
// IMAGE_VERSION currently is.
func TestTheRealImageVersionHasAnUpgradeSection(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := imageVersionProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}
}

func writeImageFixture(t *testing.T, dockerfile, upgrades string) string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, runtimeDockerfile, dockerfile)
	writeAt(t, root, runtimeUpgrades, upgrades)
	return root
}

const fixtureDockerfile = "FROM debian@sha256:aaa\nARG CLAUDE_AGENT_SDK_VERSION=0.3.233\n" +
	"ARG IMAGE_VERSION=2026.08-5\nLABEL org.opencontainers.image.version=\"${IMAGE_VERSION}\"\n"

func TestImageVersionAcceptsAVersionWithItsSection(t *testing.T) {
	t.Parallel()
	root := writeImageFixture(t, fixtureDockerfile,
		"# 升級紀錄\n\n## `2026.08-4` → `2026.08-5`（2026-08-29）\n\n四項實測全跑。\n")
	if problems := imageVersionProblems(root); len(problems) != 0 {
		t.Fatalf("a documented bump was rejected: %v", problems)
	}
}

func TestImageVersionNamesABumpNobodyWroteDown(t *testing.T) {
	t.Parallel()
	// Exactly the tree's own shape on 2026-08-29: the Dockerfile moved on, the
	// record did not.
	root := writeImageFixture(t, fixtureDockerfile,
		"# 升級紀錄\n\n## `2026.08-2` → `2026.08-3`（2026-08-16）\n\n四項實測全跑。\n")
	problems := imageVersionProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "pins IMAGE_VERSION=2026.08-5 and no heading") {
		t.Fatalf("an undocumented bump was accepted: %v", problems)
	}
}

// The version string appearing in prose is not a section. A check satisfied by a
// passing mention is satisfied by a mention that says the opposite.
func TestImageVersionWantsAHeadingAndNotAMention(t *testing.T) {
	t.Parallel()
	root := writeImageFixture(t, fixtureDockerfile,
		"# 升級紀錄\n\n## `2026.08-2` → `2026.08-3`\n\n下一版 `2026.08-5` 還沒有人跑過四項實測。\n")
	problems := imageVersionProblems(root)
	if len(problems) != 1 {
		t.Fatalf("a prose mention counted as an upgrade record: %v", problems)
	}
}

func TestImageVersionSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("no ARG at all", func(t *testing.T) {
		t.Parallel()
		root := writeImageFixture(t, "FROM debian@sha256:aaa\n# IMAGE_VERSION lived here\n", "# 升級紀錄\n")
		problems := imageVersionProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "lost its subject") {
			t.Fatalf("a Dockerfile with no IMAGE_VERSION was accepted: %v", problems)
		}
	})
	t.Run("two ARGs", func(t *testing.T) {
		t.Parallel()
		root := writeImageFixture(t, fixtureDockerfile+"ARG IMAGE_VERSION=2026.09-1\n",
			"# 升級紀錄\n\n## `2026.08-5`\n\n## `2026.09-1`\n")
		problems := imageVersionProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "the image has one version") {
			t.Fatalf("two IMAGE_VERSION declarations were accepted: %v", problems)
		}
	})
	t.Run("no UPGRADES.md", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeAt(t, root, runtimeDockerfile, fixtureDockerfile)
		if problems := imageVersionProblems(root); len(problems) == 0 {
			t.Fatal("a missing UPGRADES.md was accepted")
		}
	})
}
