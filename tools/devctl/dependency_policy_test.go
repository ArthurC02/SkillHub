package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowDependenciesNeedAPinAndAVersionComment(t *testing.T) {
	t.Parallel()
	const workflow = `jobs:
  image:
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
  build:
    services:
      db:
        image: postgres:17
      cache:
        image: "redis:7@sha256:` + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" + `"
    steps:
      - uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4
      - uses: actions/setup-go@v5
      - uses: actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9
      - uses: ./.github/actions/golangci-lint
      - name: container step
        uses: docker://alpine:3
`
	problems := workflowPinProblems("ci.yml", workflow)
	want := []string{"image postgres:17", "uses actions/setup-go@v5 is not pinned to a commit SHA", "actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9 has no # vX", "uses docker://alpine:3 is not pinned by digest"}
	if len(problems) != len(want) {
		t.Fatalf("got %d problems, want %d:\n%s", len(problems), len(want), strings.Join(problems, "\n"))
	}
	for _, fragment := range want {
		if !strings.Contains(strings.Join(problems, "\n"), fragment) {
			t.Fatalf("missing %q in:\n%s", fragment, strings.Join(problems, "\n"))
		}
	}
}

func TestDockerfileStagesDoNotNeedDigests(t *testing.T) {
	t.Parallel()
	const dockerfile = "FROM golang:1.27@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef AS build\n" +
		"FROM build AS test\n" +
		"FROM --platform=linux/amd64 nginx:1\n"
	problems := dockerfilePinProblems("Dockerfile", dockerfile)
	if len(problems) != 1 || !strings.Contains(problems[0], "FROM nginx:1") {
		t.Fatalf("got %v", problems)
	}
}

func TestNpmrcMustTurnOffInstallScripts(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"ignore-scripts=true\n":                  true,
		"audit=false\n ignore-scripts = true \n": true,
		"ignore-scripts=false\n":                 false,
		"# ignore-scripts=true\n":                false,
		"":                                       false,
	}
	for npmrc, want := range cases {
		if got := npmrcIgnoresScripts(npmrc); got != want {
			t.Errorf("npmrcIgnoresScripts(%q) = %v, want %v", npmrc, got, want)
		}
	}
}

func TestUvProjectsMustSetACooldown(t *testing.T) {
	t.Parallel()
	if !uvCooldown.MatchString("[project]\nname = \"x\"\n\n[tool.uv]\nexclude-newer = \"7 days\"\n") {
		t.Fatal("a pyproject with exclude-newer was rejected")
	}
	if uvCooldown.MatchString("[tool.uv]\n# exclude-newer = \"7 days\"\n") {
		t.Fatal("a commented-out exclude-newer was accepted")
	}
}

func TestDependabotMustListTheExactDirectory(t *testing.T) {
	t.Parallel()
	const config = "    directories:\n      - /apps/web\n      - /apps/webx\n    directory: /tools/pglite\n"
	cases := map[string]bool{"apps/web": true, "apps/webx": true, "tools/pglite": true, "apps": false, "apps/we": false, "tools": false}
	for dir, want := range cases {
		if got := dependabotCovers(config, dir); got != want {
			t.Errorf("dependabotCovers(%q) = %v, want %v", dir, got, want)
		}
	}
}

func TestComposeAndCIMustRunTheSameImage(t *testing.T) {
	t.Parallel()
	digestA, digestB := "@sha256:"+strings.Repeat("a", 64), "@sha256:"+strings.Repeat("b", 64)
	compose := map[string]string{"infra/compose/docker-compose.yml": "services:\n  db:\n    image: pgvector/pgvector:pg17" + digestA +
		"\n  s3:\n    image: chrislusf/seaweedfs:3.80" + digestA + "\n"}
	cases := []struct {
		name, workflow, drifted string
	}{
		{"the same image written with its registry", "        image: docker.io/pgvector/pgvector:pg17" + digestA, ""},
		{"another tag written with its registry", "        image: docker.io/pgvector/pgvector:pg16" + digestA, "docker.io/pgvector/pgvector:pg16"},
		{"another tag in a docker run line", "          docker run -d chrislusf/seaweedfs:4.46" + digestA + " server", "chrislusf/seaweedfs:4.46"},
		{"the same tag with another digest", "        image: pgvector/pgvector:pg17" + digestB, "pgvector/pgvector:pg17" + digestB},
		{"an image compose does not run", "        image: redis:7" + digestB, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			problems := composeAndWorkflowImageDrift(compose, map[string]string{"ci.yml": c.workflow + "\n"})
			if c.drifted == "" {
				if len(problems) != 0 {
					t.Fatalf("got %v, want no drift", problems)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], "ci.yml: "+c.drifted) || !strings.Contains(problems[0], "bump both together") {
				t.Fatalf("got %v, want one drift naming ci.yml and %s", problems, c.drifted)
			}
		})
	}
}

func TestDependencyPolicyComparesComposeWithTheWorkflows(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	digest := "@sha256:" + strings.Repeat("c", 64)
	files := map[string]string{
		".github/dependabot.yml":           "updates:\n  - package-ecosystem: docker-compose\n    directory: /infra/compose\n",
		"infra/compose/docker-compose.yml": "services:\n  db:\n    image: pgvector/pgvector:pg17" + digest + "\n",
		".github/workflows/ci.yml":         "jobs:\n  test:\n    services:\n      db:\n        image: pgvector/pgvector:pg16" + digest + "\n",
	}
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	problems := strings.Join(dependencyPolicyProblems(root), "\n")
	for _, want := range []string{".github/workflows/ci.yml: pgvector/pgvector:pg16", ".node-version: no node version found"} {
		if !strings.Contains(problems, want) {
			t.Fatalf("missing %q in:\n%s", want, problems)
		}
	}
}

func TestTheRealTreeFollowsTheDependencyPolicy(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := dependencyPolicyProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}
}
