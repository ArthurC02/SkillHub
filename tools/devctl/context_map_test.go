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
	write("docs/adr/"+contextMapADR, adr)
	write("apps/platform/.golangci.yml", lint)
	for _, name := range packages {
		write("apps/platform/internal/"+name+"/doc.go", "package "+filepath.Base(name)+"\n")
	}
	return root
}

const contextMapADRFixture = `### 1. Context 對照表

| 產品／Bounded Context | 類型 | Boundary ID | 現行 internal path | 需求 ID 前綴 |
| --- | --- | --- | --- | --- |
| Skill 試跑執行／Run Orchestration | Core | run | run | RUN |
| Skill 接納與信任／Trust & Supply Chain | Core | ingest | ingest | SKILL |
| — | Shared Kernel | skillpkg | shared/skillpkg | — |
| — | Generic | audit | foundation/observability/audit | — |
| — | Generic | queue | foundation/messaging/queue | — |
| — | Generic | platform | foundation/persistence/db/gen | — |
| — | Generic | apiserver | entrypoint/api/apiserver | — |
| — | Generic | api | entrypoint/api/gen | — |

### 2. Governance
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
	nestedADR := strings.Replace(contextMapADRFixture, "| run | run | RUN |", "| run | trial/execution | RUN |", 1)
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
			name: "overlapping selectors are rejected",
			adr: strings.Replace(nestedADR, "\n### 2.", "\n| 執行證據／Run Trace | Supporting | trace | trial/* | TRACE |\n\n### 2.", 1),
			lint:     nestedLint,
			packages: nestedPackages,
			want:     `internal paths "trial/execution" (run) and "trial/*" (trace) overlap`,
		},
		{
			name: "duplicate Boundary ID is rejected",
			adr: strings.Replace(contextMapADRFixture, "\n### 2.", "\n| 重複 | Core | run | duplicate | RUN |\n\n### 2.", 1),
			lint:     contextMapLintFixture,
			packages: flatPackages,
			want:     `declares Boundary ID "run" twice`,
		},
		{
			name: "duplicate path is rejected",
			adr: strings.Replace(contextMapADRFixture, "\n### 2.", "\n| 重複 | Core | trace | run | TRACE |\n\n### 2.", 1),
			lint:     contextMapLintFixture,
			packages: flatPackages,
			want:     `declares internal path "run" twice (run and trace)`,
		},
		{
			name: "stale nested depguard glob is rejected",
			adr:      nestedADR,
			lint:     strings.Replace(nestedLint, "              - \"!$test\"", "              - \"**/internal/ghost/nested/**\"\n              - \"!$test\"", 1),
			packages: nestedPackages,
			want:     "guards apps/platform/internal/ghost/nested but no ADR-032 §1 Boundary ID declares that path",
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
