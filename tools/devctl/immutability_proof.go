package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const immutabilityTestPath = "db/tests/immutability_test.sql"

var (
	immutableTriggerCreate = regexp.MustCompile(`CREATE\s+TRIGGER\s+(\w+_immutable)\s+BEFORE\s+([A-Z][A-Z ]*?)\s+ON\s+(\w+)`)
	immutableTriggerDrop   = regexp.MustCompile(`DROP\s+TRIGGER\s+(?:IF\s+EXISTS\s+)?(\w+_immutable)\s+ON\s+\w+`)
)

type immutableTrigger struct {
	table string
	ops   string
}

func liveImmutableTriggers(root string) (map[string]immutableTrigger, []string) {
	paths, err := filepath.Glob(filepath.Join(root, "db", "migrations", "*.sql"))
	if err != nil {
		return nil, []string{fmt.Sprintf("immutability-proof: cannot list db/migrations: %v", err)}
	}
	sort.Strings(paths)

	live := map[string]immutableTrigger{}
	var problems []string
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("immutability-proof: cannot read %s: %v", path, err))
			continue
		}
		for _, statement := range strings.Split(string(body), ";") {
			if m := immutableTriggerDrop.FindStringSubmatch(statement); m != nil {
				delete(live, m[1])
			}
			if m := immutableTriggerCreate.FindStringSubmatch(statement); m != nil {
				live[m[1]] = immutableTrigger{table: m[3], ops: m[2]}
			}
		}
	}
	return live, problems
}

func immutabilityProofProblems(root string) []string {
	live, problems := liveImmutableTriggers(root)
	if len(live) == 0 {
		return append(problems, "immutability-proof: db/migrations declares no *_immutable trigger; this check has lost its subject")
	}

	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(immutabilityTestPath)))
	if err != nil {
		return append(problems, fmt.Sprintf("immutability-proof: cannot read %s: %v", immutabilityTestPath, err))
	}
	test := string(body)

	names := make([]string, 0, len(live))
	for name := range live {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		trigger := live[name]
		for _, op := range []string{"UPDATE", "DELETE"} {
			if !strings.Contains(trigger.ops, op) {
				continue
			}
			target := regexp.QuoteMeta(trigger.table)
			attempt := `must_\w+\(\$\$\s*UPDATE\s+` + target + `\b`
			if op == "DELETE" {
				attempt = `must_\w+\(\$\$\s*DELETE\s+FROM\s+` + target + `\b`
			}
			if regexp.MustCompile(attempt).MatchString(test) {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"immutability-proof: trigger %s guards %s on %s, but %s never attempts a %s on it inside a must_* helper. "+
					"A guard with no case against it is an assumption, not a proof.",
				name, op, trigger.table, immutabilityTestPath, op))
		}
	}
	return problems
}
