package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeContextMapFixture(t *testing.T, adr, lint string, packages []string) string {
	t.Helper()
	root := t.TempDir()
	write := func(relative, contents string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(contextMapDoc, adr)
	write("apps/platform/.golangci.yml", lint)
	for _, name := range packages {
		write("apps/platform/internal/"+name+"/doc.go", "package "+filepath.Base(name)+"\n")
	}
	return root
}

const contextMapADRFixture = `packages:
  - id: run
    kind: Core
    path: run
    context: Skill 試跑執行／Run Orchestration
  - id: ingest
    kind: Core
    path: ingest
    context: Skill 接納與信任／Trust & Supply Chain
  - id: skillpkg
    kind: Shared Kernel
    path: shared/skillpkg
  - id: audit
    kind: Generic
    path: foundation/observability/audit
  - id: queue
    kind: Generic
    path: foundation/messaging/queue
  - id: platform
    kind: Generic
    path: foundation/persistence/db/gen
  - id: apiserver
    kind: Generic
    path: entrypoint/api/apiserver
  - id: api
    kind: Generic
    path: entrypoint/api/gen
`

const contextMapLintFixture = `      depguard:
        rules:
          run:
            files:
              - "**/internal/run/**"
              - "!$test"
          ingest:
            files:
              - "**/internal/ingest/**"
              - "!$test"
          generic:
            files:
              - "**/internal/foundation/observability/audit/**"
              - "**/internal/foundation/messaging/queue/**"
              - "**/internal/foundation/persistence/db/gen/**"
              - "!$test"
          shared-kernel:
            files:
              - "**/internal/shared/skillpkg/**"
              - "!$test"
`

func TestContextMapProblems(t *testing.T) {
	t.Parallel()

	flatPackages := []string{"run", "ingest", "shared/skillpkg", "foundation/observability/audit", "foundation/messaging/queue", "foundation/persistence/db/gen", "entrypoint/api/apiserver", "entrypoint/api/gen"}
	nestedADR := strings.Replace(contextMapADRFixture, "    path: run\n", "    path: trial/execution\n", 1)
	nestedLint := strings.Replace(contextMapLintFixture, "**/internal/run/**", "**/internal/trial/execution/**", 1)
	nestedPackages := []string{"trial/execution", "ingest", "shared/skillpkg", "foundation/observability/audit", "foundation/messaging/queue", "foundation/persistence/db/gen", "entrypoint/api/apiserver", "entrypoint/api/gen"}

	tests := []struct {
		name     string
		adr      string
		lint     string
		packages []string
		want     string
	}{
		{
			name:     "flat layout remains compatible",
			adr:      contextMapADRFixture,
			lint:     contextMapLintFixture,
			packages: flatPackages,
		},
		{
			name:     "nested layout is complete",
			adr:      nestedADR,
			lint:     nestedLint,
			packages: nestedPackages,
		},
		{
			name:     "unknown nested package is rejected",
			adr:      nestedADR,
			lint:     nestedLint,
			packages: append(append([]string{}, nestedPackages...), "trial/evidence"),
			want:     "apps/platform/internal/trial/evidence is not listed",
		},
		{
			name:     "overlapping selectors are rejected",
			adr:      nestedADR + "  - id: trace\n    kind: Supporting\n    path: trial/*\n    context: 執行證據／Run Trace\n",
			lint:     nestedLint,
			packages: nestedPackages,
			want:     `internal paths "trial/execution" (run) and "trial/*" (trace) overlap`,
		},
		{
			name:     "duplicate Boundary ID is rejected",
			adr:      contextMapADRFixture + "  - id: run\n    kind: Core\n    path: duplicate\n    context: 重複\n",
			lint:     contextMapLintFixture,
			packages: flatPackages,
			want:     `declares Boundary ID "run" twice`,
		},
		{
			name:     "duplicate path is rejected",
			adr:      contextMapADRFixture + "  - id: trace\n    kind: Core\n    path: run\n    context: 重複\n",
			lint:     contextMapLintFixture,
			packages: flatPackages,
			want:     `declares internal path "run" twice (run and trace)`,
		},
		{

			name: "a commented-out rule does not count as coverage",
			adr:  contextMapADRFixture,
			lint: strings.Replace(contextMapLintFixture,
				`              - "**/internal/ingest/**"`,
				"#             - \"**/internal/ingest/**\"", 1),
			packages: flatPackages,
			want:     `has no depguard rule covering internal/ingest`,
		},
		{

			name: "a path named only in a comment does not count as coverage",
			adr:  contextMapADRFixture,
			lint: strings.Replace(contextMapLintFixture,
				`              - "**/internal/ingest/**"`,
				"              # ingest lives at **/internal/ingest/** until DDD-038", 1),
			packages: flatPackages,
			want:     `has no depguard rule covering internal/ingest`,
		},
		{
			name:     "stale nested depguard glob is rejected",
			adr:      nestedADR,
			lint:     strings.Replace(nestedLint, "              - \"!$test\"", "              - \"**/internal/ghost/nested/**\"\n              - \"!$test\"", 1),
			packages: nestedPackages,
			want:     "guards apps/platform/internal/ghost/nested but no Boundary ID in " + contextMapDoc + " declares that path",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := writeContextMapFixture(t, test.adr, test.lint, test.packages)
			problems := contextMapProblems(root)
			if test.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %#v", problems)
				}
				return
			}
			for _, problem := range problems {
				if strings.Contains(problem, test.want) {
					return
				}
			}
			t.Fatalf("no problem mentions %q, got %#v", test.want, problems)
		})
	}
}
