package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
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
		committed, err := committedProblems(root, rangeSpec, out)
		if err != nil {
			return err
		}
		problems = append(problems, committed...)
	}
	if !hook {
		if err := automationCheck(root, out); err != nil {
			problems = append(problems, "automation-check reads the working tree, so a FAIL above in a file you did not commit is someone else's work in progress: "+err.Error())
		}
	}

	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(out, statusFail, problem)
		}
		return errors.New("preflight found what CI would fail on")
	}
	fmt.Fprintf(out, "preflight: %s clean\n", strings.Join(ranges, ", "))
	return nil
}

func committedProblems(root, rangeSpec string, out io.Writer) ([]string, error) {
	format, warnings, err := formatProblems(root, rangeSpec)
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		fmt.Fprintln(out, statusWarn, warning)
	}
	comments, err := committedCommentProblems(root, rangeSpec)
	if err != nil {
		return nil, err
	}
	evidence, evidenceWarnings, err := committedEvidenceProblems(root, rangeSpec)
	if err != nil {
		return nil, err
	}
	for _, warning := range evidenceWarnings {
		fmt.Fprintln(out, statusWarn, warning)
	}
	bump, err := imageBumpProblems(root, rangeEnd(rangeSpec))
	if err != nil {
		return nil, err
	}
	guards, guardsWarnings, err := committedGuardsTestProblems(root, rangeSpec, guardsTestCommand)
	if err != nil {
		return nil, err
	}
	for _, warning := range guardsWarnings {
		fmt.Fprintln(out, statusWarn, warning)
	}
	advice, err := committedDecisionCommentWarnings(root, rangeSpec, newDecisionJudge(root))
	if err != nil {
		return nil, err
	}
	for _, warning := range advice {
		fmt.Fprintln(out, statusWarn, warning)
	}
	return append(append(append(append(format, comments...), evidence...), bump...), guards...), nil
}

func unpushedRange(root string) string {
	if _, err := gitOutput(root, "rev-parse", "--verify", "--quiet", "@{upstream}"); err == nil {
		return "@{upstream}..HEAD"
	}
	return "origin/main..HEAD"
}

const prePushInputFields = 4

func prePushRanges(root string, in io.Reader) ([]string, error) {
	var ranges []string
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		if len(fields) != prePushInputFields {
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
		blob, err := gitOutput(root, "cat-file", "blob", end+":"+file)
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

func committedCommentProblems(root, rangeSpec string) ([]string, error) {
	names, err := gitOutput(root, "diff", "--name-only", "--diff-filter=ACMR", rangeSpec)
	if err != nil {
		return nil, err
	}
	end := rangeEnd(rangeSpec)
	var problems []string
	for _, file := range lines(names) {
		prefixes, ok := commentPrefixesFor(path.Base(file))
		if !ok || !commentBudgetCovers(file) {
			continue
		}
		blob, err := gitOutput(root, "cat-file", "blob", end+":"+file)
		if err != nil {
			return nil, err
		}
		if found := commentViolationsIn([]byte(blob), prefixes); len(found) > 0 {
			problems = append(problems, commentBudgetProblem(file, len(found)))
		}
	}
	return problems, nil
}

func formatterFor(root, file string) (formatter, bool) {
	switch {
	case strings.HasSuffix(file, extGo) && (strings.HasPrefix(file, dirAppsPlatform+"/") || strings.HasPrefix(file, dirAppsSandbox+"/")):
		return formatter{name: "gofmt", cmd: "gofmt", fix: "gofmt -w " + file}, true
	case strings.HasPrefix(file, dirAppsWeb+"/"):
		prettier := filepath.Join(root, "apps", "web", "node_modules", "prettier", "bin", "prettier.cjs")
		return formatter{
			name: "prettier", dir: dirAppsWeb, cmd: "node",
			args:     []string{prettier, "--stdin-filepath", strings.TrimPrefix(file, dirAppsWeb+"/")},
			requires: prettier, fix: "task format:web",
		}, true
	case strings.HasPrefix(file, dirAppsLLM+"/") && strings.HasSuffix(file, extPy):
		return formatter{
			name: toolRuff, dir: dirAppsLLM, cmd: "uv",
			args: []string{cmdRun, "--no-sync", toolRuff, "format", "--stdin-filename", strings.TrimPrefix(file, dirAppsLLM+"/"), "-"},
			fix:  "task format:llm",
		}, true
	}
	return formatter{}, false
}

func (f formatter) format(root, source string) (string, error) {
	if _, err := exec.LookPath(f.cmd); err != nil {
		return "", fmt.Errorf("%w: %w", errFormatterUnavailable, err)
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
		return "", fmt.Errorf("%w: %s", err, firstLine(strings.TrimSpace(stderr.String())))
	}
	return stdout.String(), nil
}
