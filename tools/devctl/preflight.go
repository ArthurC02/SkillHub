package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	errFormatterUnavailable = errors.New("formatter unavailable")
	errNotFormattable       = errors.New("formatter has no parser for this file")
)

type formatter struct {
	name     string
	dir      string
	cmd      string
	args     []string
	requires string
	fix      string
}

func preflight(root string, args []string, in io.Reader, out io.Writer) error {
	hook := len(args) == 1 && args[0] == "--hook"
	if len(args) > 0 && !hook {
		return errors.New("usage: devctl preflight [--hook]")
	}
	var ranges []string
	if hook {
		parsed, err := prePushRanges(root, in)
		if err != nil {
			return err
		}
		ranges = parsed
	} else {
		ranges = []string{unpushedRange(root)}
	}

	var problems []string
	for _, rangeSpec := range ranges {
		bump, err := imageBumpProblemsInRange(root, rangeSpec)
		if err != nil {
			return err
		}
		problems = append(problems, bump...)
		format, warnings, err := formatProblems(root, rangeSpec)
		if err != nil {
			return err
		}
		for _, warning := range warnings {
			fmt.Fprintln(out, "WARN", warning)
		}
		problems = append(problems, format...)
	}
	if !hook {
		if err := automationCheck(root, out); err != nil {
			problems = append(problems, "automation-check reads the working tree, so a FAIL above in a file you did not commit is someone else's work in progress: "+err.Error())
		}
	}

	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(out, "FAIL", problem)
		}
		return errors.New("preflight found what CI would fail on")
	}
	fmt.Fprintf(out, "preflight: %s clean\n", strings.Join(ranges, ", "))
	return nil
}

func unpushedRange(root string) string {
	if _, err := gitOutput(root, "rev-parse", "--verify", "--quiet", "@{upstream}"); err == nil {
		return "@{upstream}..HEAD"
	}
	return "origin/main..HEAD"
}

func prePushRanges(root string, in io.Reader) ([]string, error) {
	var ranges []string
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 4 {
			return nil, fmt.Errorf("pre-push: unexpected input line %q", scanner.Text())
		}
		local, remote := fields[1], fields[3]
		switch {
		case isZeroObject(local):
		case isZeroObject(remote):
			ranges = append(ranges, "origin/main.."+local)
		default:
			if _, err := gitOutput(root, "cat-file", "-e", remote+"^{commit}"); err != nil {
				return nil, fmt.Errorf("pre-push: the remote is at %s, which this clone does not have; fetch first", remote)
			}
			ranges = append(ranges, remote+".."+local)
		}
	}
	return ranges, scanner.Err()
}

func isZeroObject(sha string) bool {
	return strings.Trim(sha, "0") == ""
}

func formatProblems(root, rangeSpec string) (problems, warnings []string, err error) {
	names, err := gitOutput(root, "diff", "--name-only", "--diff-filter=ACMR", rangeSpec)
	if err != nil {
		return nil, nil, err
	}
	end := rangeEnd(rangeSpec)
	unavailable := map[string]bool{}
	for _, file := range lines(names) {
		f, ok := formatterFor(root, file)
		if !ok || unavailable[f.name] {
			continue
		}
		blob, err := gitOutput(root, "show", end+":"+file)
		if err != nil {
			return nil, nil, err
		}
		formatted, err := f.format(root, blob)
		switch {
		case errors.Is(err, errFormatterUnavailable):
			unavailable[f.name] = true
			warnings = append(warnings, fmt.Sprintf("%s skipped (%v); CI still checks it", f.name, err))
		case errors.Is(err, errNotFormattable):
		case err != nil:
			warnings = append(warnings, fmt.Sprintf("%s could not check %s (%v); CI still checks it", f.name, file, err))
		case formatted != blob:
			problems = append(problems, fmt.Sprintf("%s is not %s-formatted as committed; run `%s` and commit the result", file, f.name, f.fix))
		}
	}
	return problems, warnings, nil
}

func formatterFor(root, file string) (formatter, bool) {
	switch {
	case strings.HasSuffix(file, ".go") && (strings.HasPrefix(file, "apps/platform/") || strings.HasPrefix(file, "apps/sandbox/")):
		return formatter{name: "gofmt", cmd: "gofmt", fix: "gofmt -w " + file}, true
	case strings.HasPrefix(file, "apps/web/"):
		prettier := filepath.Join(root, "apps", "web", "node_modules", "prettier", "bin", "prettier.cjs")
		return formatter{
			name: "prettier", dir: "apps/web", cmd: "node",
			args:     []string{prettier, "--stdin-filepath", strings.TrimPrefix(file, "apps/web/")},
			requires: prettier, fix: "task format:web",
		}, true
	case strings.HasPrefix(file, "apps/llm/") && strings.HasSuffix(file, ".py"):
		return formatter{
			name: "ruff", dir: "apps/llm", cmd: "uv",
			args: []string{"run", "--no-sync", "ruff", "format", "--stdin-filename", strings.TrimPrefix(file, "apps/llm/"), "-"},
			fix:  "task format:llm",
		}, true
	}
	return formatter{}, false
}

func (f formatter) format(root, source string) (string, error) {
	if _, err := exec.LookPath(f.cmd); err != nil {
		return "", fmt.Errorf("%w: %v", errFormatterUnavailable, err)
	}
	if f.requires != "" && !fileExists(f.requires) {
		return "", fmt.Errorf("%w: %s is missing, run task bootstrap", errFormatterUnavailable, f.requires)
	}
	cmd := exec.Command(f.cmd, f.args...)
	cmd.Dir = filepath.Join(root, filepath.FromSlash(f.dir))
	cmd.Stdin = strings.NewReader(source)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "No parser could be inferred") {
			return "", errNotFormattable
		}
		return "", fmt.Errorf("%v: %s", err, firstLine(strings.TrimSpace(stderr.String())))
	}
	return stdout.String(), nil
}
