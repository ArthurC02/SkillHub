package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const pinnedBase = "node:22@sha256:d649c27dae7ba0137b3cef5dd75baa422c08dc3d9e3fc0c23dfb172dc3cc6436"

const gateDockerfile = "FROM " + pinnedBase + "\n" +
	"COPY constraints.txt /tmp/constraints.txt\n" +
	"COPY package.json package-lock.json /opt/skillhub/\n" +
	"COPY run.mjs /opt/skillhub/run.mjs\n"

func TestTheRealRuntimeDockerfileIsPinnedAndItsInputsAreKnown(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := digestPinProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runtimeDockerfile)))
	if err != nil {
		t.Fatal(err)
	}
	inputs := runtimeImageInputs(string(data))
	if inputs.everything || !inputs.include("run.mjs") || inputs.include("run.test.mjs") {
		t.Fatalf("the real Dockerfile's inputs were misread: %+v", inputs)
	}
}

func TestImageBumpOnlyAsksForAVersionWhenTheImageIsBuiltFromTheChange(t *testing.T) {
	t.Parallel()
	const bumped = "-ARG IMAGE_VERSION=2026.08-10\n+ARG IMAGE_VERSION=2026.08-11\n"
	cases := []struct {
		name    string
		changed []string
		diff    string
		fail    bool
	}{
		{"a copied file without a bump", []string{"run.mjs"}, "", true},
		{"a copied file with a bump", []string{"run.mjs", "UPGRADES.md"}, bumped, false},
		{"the Dockerfile itself without a bump", []string{"Dockerfile"}, "+# note\n", true},
		{"a test file no COPY names", []string{"run.test.mjs"}, "", false},
		{"only the upgrade record", []string{"UPGRADES.md"}, "", false},
		{"one of several sources in a single COPY", []string{"package-lock.json"}, "", true},
		{"a copied file next to a test file", []string{"run.test.mjs", "constraints.txt"}, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := imageBumpProblems(tc.changed, gateDockerfile, tc.diff)
			if tc.fail != (len(problems) == 1) || len(problems) > 1 {
				t.Fatalf("changed=%v diff=%q: got %v", tc.changed, tc.diff, problems)
			}
			if tc.fail && !strings.HasPrefix(problems[0], "I-05: ") {
				t.Fatalf("the failure does not name its gate: %q", problems[0])
			}
		})
	}
}

func TestImageInputsFailClosedOnSourcesThatAreNotPlainPaths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		dockerfile string
		file       string
		include    bool
	}{
		{"a directory source covers its files", "COPY src ./src\n", "src/a.mjs", true},
		{"a directory source does not cover a sibling prefix", "COPY src ./src\n", "srcx/a.mjs", false},
		{"a leading ./ is the same path", "COPY ./run.mjs /x\n", "run.mjs", true},
		{"the whole context", "COPY . /app\n", "notes.txt", true},
		{"a glob", "COPY *.mjs /app/\n", "helper.txt", true},
		{"the JSON form", `COPY ["run.mjs", "/x"]` + "\n", "anything", true},
		{"a copy from another stage reads no context", "COPY --from=build /out /out\n", "out", false},
		{"a flag before the sources", "COPY --chown=1000:1000 run.mjs /x\n", "run.mjs", true},
		{"a continued instruction", "COPY a.txt \\\n  b.txt /x/\n", "b.txt", true},
		{"a remote ADD reads no context", "ADD https://example.com/x.tgz /x\n", "x.tgz", false},
		{"a commented-out COPY", "# COPY secret.txt /x\n", "secret.txt", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := runtimeImageInputs(tc.dockerfile).include(tc.file); got != tc.include {
				t.Fatalf("%q includes %q = %v, want %v", tc.dockerfile, tc.file, got, tc.include)
			}
		})
	}
}

func TestBaseImagesMustBePinnedByDigestUnlessTheyAreAnEarlierStage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		dockerfile string
		problems   int
	}{
		{"pinned", "FROM " + pinnedBase + "\n", 0},
		{"pinned with a platform flag and a stage name", "FROM --platform=linux/amd64 " + pinnedBase + " AS build\n", 0},
		{"a tag only", "FROM node:22-bookworm-slim\n", 1},
		{"a digest one hex digit short", "FROM node@sha256:" + strings.Repeat("a", 63) + "\n", 1},
		{"an earlier stage", "FROM " + pinnedBase + " AS build\nFROM build\n", 0},
		{"a second, unpinned base", "FROM " + pinnedBase + " AS build\nFROM debian:12\n", 1},
		{"no FROM at all", "RUN true\n", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := unpinnedBaseImages(tc.dockerfile); len(got) != tc.problems {
				t.Fatalf("want %d problems, got %v", tc.problems, got)
			}
		})
	}
}

func TestImageBumpReadsTheRangeFromGit(t *testing.T) {
	t.Parallel()
	repo := newGitRepo(t)
	repo.commit(t, map[string]string{
		runtimeDockerfile:                    gateDockerfile + "ARG IMAGE_VERSION=2026.08-10\n",
		runtimeImageDir + "/run.mjs":         "export {}\n",
		runtimeImageDir + "/run.test.mjs":    "test()\n",
		runtimeImageDir + "/constraints.txt": "x==1\n",
	})
	base := repo.head(t)

	repo.commit(t, map[string]string{runtimeImageDir + "/run.test.mjs": "test(1)\n"})
	if problems, err := imageBumpProblemsInRange(repo.root, base+"..HEAD"); err != nil || len(problems) != 0 {
		t.Fatalf("a test-only change asked for a bump: %v %v", problems, err)
	}

	repo.commit(t, map[string]string{runtimeImageDir + "/run.mjs": "export const x = 1\n"})
	problems, err := imageBumpProblemsInRange(repo.root, base+"..HEAD")
	if err != nil || len(problems) != 1 || !strings.Contains(problems[0], "run.mjs") {
		t.Fatalf("an unbumped change to a copied file passed: %v %v", problems, err)
	}
}

func TestRangeEndNamesTheCommitBeingChecked(t *testing.T) {
	t.Parallel()
	for spec, want := range map[string]string{
		"a..b":   "b",
		"a...b":  "b",
		"a..":    "HEAD",
		"abc123": "abc123",
	} {
		if got := rangeEnd(spec); got != want {
			t.Errorf("rangeEnd(%q) = %q, want %q", spec, got, want)
		}
	}
}
