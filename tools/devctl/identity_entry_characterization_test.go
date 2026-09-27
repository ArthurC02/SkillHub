package main

import (
	"slices"
	"testing"
)

const identityEntryBaseFixture = "packages:\n  - id: run\n    kind: Core\n    path: run\n    context: Run\n"

func TestIdentityEntryShapeRules(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name         string
		entry        string
		id           string
		wantProblems []string
	}{
		{
			name:  "a Generic package without a context is declared",
			entry: "  - id: audit\n    kind: Generic\n    path: foundation/audit\n",
			id:    "audit",
		},
		{
			name:  "a Shared Kernel package without a context is declared",
			entry: "  - id: skillpkg\n    kind: Shared Kernel\n    path: shared/skillpkg\n",
			id:    "skillpkg",
		},
		{
			name:         "an unknown architecture kind is rejected",
			entry:        "  - id: audit\n    kind: Mystery\n    path: foundation/audit\n",
			id:           "audit",
			wantProblems: []string{`apps/platform/architecture-identity.yaml has unknown architecture kind "Mystery"`},
		},
		{
			name:         "a Bounded Context without a context name is rejected",
			entry:        "  - id: trace\n    kind: Supporting\n    path: trace\n",
			id:           "trace",
			wantProblems: []string{`docs/domain-memory/registry/contexts.json kind "Supporting" requires a context name`},
		},
		{
			name:         "a Generic package that names a context is rejected",
			entry:        "  - id: audit\n    kind: Generic\n    path: foundation/audit\n    context: Audit\n",
			id:           "audit",
			wantProblems: []string{`apps/platform/architecture-identity.yaml kind "Generic" must not name a Bounded Context`},
		},
		{
			name:         "a Shared Kernel package that names a context is rejected",
			entry:        "  - id: skillpkg\n    kind: Shared Kernel\n    path: shared/skillpkg\n    context: Packages\n",
			id:           "skillpkg",
			wantProblems: []string{`apps/platform/architecture-identity.yaml kind "Shared Kernel" must not name a Bounded Context`},
		},
		{
			name:         "a Boundary ID with an uppercase letter is rejected",
			entry:        "  - id: Audit\n    kind: Generic\n    path: foundation/audit\n",
			id:           "Audit",
			wantProblems: []string{`apps/platform/architecture-identity.yaml has invalid Boundary ID "Audit"`},
		},
		{
			name:         "an internal path with an uppercase segment is rejected",
			entry:        "  - id: audit\n    kind: Generic\n    path: Foundation/audit\n",
			id:           "audit",
			wantProblems: []string{`apps/platform/architecture-identity.yaml has invalid internal path "Foundation/audit"`},
		},
		{
			name:         "only the first broken rule of an entry is reported",
			entry:        "  - id: Audit\n    kind: Generic\n    path: Foundation/audit\n    context: Audit\n",
			id:           "Audit",
			wantProblems: []string{`apps/platform/architecture-identity.yaml kind "Generic" must not name a Bounded Context`},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			declared, problems := identityEntriesAsIdentities(identityEntryBaseFixture + test.entry)
			if !slices.Equal(problems, test.wantProblems) {
				t.Fatalf("problems = %#v, want %#v", problems, test.wantProblems)
			}
			_, gotDeclared := declared[test.id]
			if wantDeclared := len(test.wantProblems) == 0; gotDeclared != wantDeclared {
				t.Fatalf("Boundary ID %q declared = %v, want %v", test.id, gotDeclared, wantDeclared)
			}
			if _, ok := declared["run"]; !ok {
				t.Fatalf("the valid base entry run was not declared: %#v", declared)
			}
		})
	}
}
