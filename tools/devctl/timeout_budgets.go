package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const timeoutBudgetMargin = 5 * time.Second

var (
	timeoutOverMarker    = regexp.MustCompile(`//\s*budget-over:\s*([A-Za-z0-9_.]+)`)
	timeoutCeilingMarker = regexp.MustCompile(`#\s*budget-ceiling:\s*([A-Za-z0-9_.]+)`)

	goDurationValue = regexp.MustCompile(`(\d+)\s*\*\s*time\.(Second|Minute|Millisecond|Hour)`)

	pyFloatValue = regexp.MustCompile(`=\s*([0-9][0-9_]*(?:\.[0-9]+)?)\s*(?:#.*)?$`)
)

var goDurationUnits = map[string]time.Duration{
	"Millisecond": time.Millisecond,
	"Second":      time.Second,
	"Minute":      time.Minute,
	"Hour":        time.Hour,
}

var (
	timeoutGoRoots = []string{"apps/platform", "apps/sandbox"}
	timeoutPyRoots = []string{"apps/llm/src"}
)

type timeoutSite struct {
	file  string
	line  int
	value time.Duration
}

func timeoutBudgetProblems(root string) []string {
	over, problems := scanTimeoutMarkers(root, timeoutGoRoots, ".go", timeoutOverMarker, parseGoDuration)
	ceiling, more := scanTimeoutMarkers(root, timeoutPyRoots, ".py", timeoutCeilingMarker, parsePySeconds)
	problems = append(problems, more...)

	names := map[string]bool{}
	for name := range over {
		names[name] = true
	}
	for name := range ceiling {
		names[name] = true
	}
	var sorted []string
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	for _, name := range sorted {
		client, server := over[name], ceiling[name]
		switch {
		case len(server) == 0:
			problems = append(problems, fmt.Sprintf(
				"timeout-budget: %s is marked `budget-over` at %s but no `# budget-ceiling: %s` names it "+
					"in %s; a one-sided marker protects nothing. Mark the Python constant it has to "+
					"outlive, or drop the marker",
				name, whereTimeout(client), name, strings.Join(timeoutPyRoots, ", ")))
			continue
		case len(client) == 0:
			problems = append(problems, fmt.Sprintf(
				"timeout-budget: %s is marked `budget-ceiling` at %s but no `// budget-over: %s` names it "+
					"in %s; nothing is holding the Go deadline above it, and a Go deadline that expires "+
					"first stops Go waiting without stopping the gateway or its bill",
				name, whereTimeout(server), name, strings.Join(timeoutGoRoots, ", ")))
			continue
		}

		if len(server) > 1 {
			problems = append(problems, fmt.Sprintf(
				"timeout-budget: %s is marked `budget-ceiling` at %d sites (%s); a budget has one ceiling",
				name, len(server), whereTimeout(server)))
			continue
		}
		for _, c := range client {
			if c.value < server[0].value+timeoutBudgetMargin {
				problems = append(problems, fmt.Sprintf(
					"timeout-budget: %s:%d gives %s %s while %s:%d sets the ceiling at %s; Go must be at "+
						"least %s above Python. Below that, Go gives up first — it records a timeout, the "+
						"gateway keeps running and keeps billing, and the answer is discarded",
					c.file, c.line, name, c.value, server[0].file, server[0].line, server[0].value,
					timeoutBudgetMargin))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func whereTimeout(sites []timeoutSite) string {
	var out []string
	for _, s := range sites {
		out = append(out, fmt.Sprintf("%s:%d", s.file, s.line))
	}
	return strings.Join(out, ", ")
}

func scanTimeoutMarkers(root string, trees []string, ext string, marker *regexp.Regexp,
	parse func(string) (time.Duration, bool),
) (map[string][]timeoutSite, []string) {
	found := map[string][]timeoutSite{}
	var problems []string
	for _, tree := range trees {
		base := filepath.Join(root, filepath.FromSlash(tree))
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				switch d.Name() {
				case ".venv", "node_modules", "gen", "generated", "__pycache__":
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ext || strings.HasSuffix(path, "_test"+ext) {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				relative = path
			}
			relative = filepath.ToSlash(relative)
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				m := marker.FindStringSubmatch(line)
				if m == nil {
					continue
				}
				value, ok := parse(line)
				at := i + 1
				if !ok {

					for j := i + 1; j < len(lines) && j <= i+2; j++ {
						if strings.TrimSpace(lines[j]) == "" {
							continue
						}
						value, ok = parse(lines[j])
						at = j + 1
						break
					}
				}
				if !ok {
					problems = append(problems, fmt.Sprintf(
						"timeout-budget: %s:%d marks %s but neither that line nor the next carries a "+
							"duration this check can read (Go: `N * time.Second`; Python: `NAME = N.N`)",
						relative, i+1, m[1]))
					continue
				}
				found[m[1]] = append(found[m[1]], timeoutSite{file: relative, line: at, value: value})
			}
			return nil
		})
	}
	return found, problems
}

func parseGoDuration(line string) (time.Duration, bool) {
	m := goDurationValue.FindStringSubmatch(line)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return time.Duration(n) * goDurationUnits[m[2]], true
}

func parsePySeconds(line string) (time.Duration, bool) {
	m := pyFloatValue.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
	if m == nil {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(strings.ReplaceAll(m[1], "_", ""), 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}
