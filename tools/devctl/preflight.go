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
	"runtime"
	"strconv"
	"strings"
	"sync"
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

type stepFindings struct {
	problems, warnings []string
	err                error
}

func committedProblems(root, rangeSpec string, out io.Writer) ([]string, error) {
	steps := []func() stepFindings{
		func() stepFindings {
			problems, warnings, err := formatProblems(root, rangeSpec)
			return stepFindings{problems, warnings, err}
		},
		func() stepFindings {
			problems, err := committedCommentProblems(root, rangeSpec)
			return stepFindings{problems: problems, err: err}
		},
		func() stepFindings {
			problems, warnings, err := committedEvidenceProblems(root, rangeSpec)
			return stepFindings{problems, warnings, err}
		},
		func() stepFindings {
			problems, err := imageBumpProblems(root, rangeEnd(rangeSpec))
			return stepFindings{problems: problems, err: err}
		},
		func() stepFindings {
			problems, warnings, err := committedGuardsTestProblems(root, rangeSpec, guardsTestCommand)
			return stepFindings{problems, warnings, err}
		},
		func() stepFindings {
			warnings, err := committedDecisionCommentWarnings(root, rangeSpec, newDecisionJudge(root))
			return stepFindings{warnings: warnings, err: err}
		},
	}
	findings := make([]stepFindings, len(steps))
	var wg sync.WaitGroup
	for i, step := range steps {
		wg.Go(func() { findings[i] = step() })
	}
	wg.Wait()

	var problems []string
	for _, found := range findings {
		if found.err != nil {
			return nil, found.err
		}
		for _, warning := range found.warnings {
			fmt.Fprintln(out, statusWarn, warning)
		}
		problems = append(problems, found.problems...)
	}
	return problems, nil
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
	type formatCheck struct {
		file        string
		f           formatter
		unavailable error
		formatted   string
		err         error
	}
	var checks []*formatCheck
	var files []string
	availability := map[string]error{}
	for _, file := range lines(names) {
		f, ok := formatterFor(root, file)
		if !ok {
			continue
		}
		unavailable, seen := availability[f.name]
		if !seen {
			unavailable = f.availability()
			availability[f.name] = unavailable
			if unavailable != nil {
				checks = append(checks, &formatCheck{f: f, unavailable: unavailable})
			}
		}
		if unavailable == nil {
			checks = append(checks, &formatCheck{file: file, f: f})
			files = append(files, file)
		}
	}
	blobs, err := committedBlobs(root, rangeEnd(rangeSpec), files)
	if err != nil {
		return nil, nil, err
	}
	var wg sync.WaitGroup
	workers := make(chan struct{}, runtime.NumCPU())
	for _, check := range checks {
		if check.unavailable != nil {
			continue
		}
		wg.Go(func() {
			workers <- struct{}{}
			defer func() { <-workers }()
			check.formatted, check.err = check.f.format(root, blobs[check.file])
		})
	}
	wg.Wait()

	for _, check := range checks {
		switch {
		case check.unavailable != nil:
			warnings = append(warnings, fmt.Sprintf("%s skipped (%v); CI still checks it", check.f.name, check.unavailable))
		case errors.Is(check.err, errNotFormattable):
		case check.err != nil:
			warnings = append(warnings, fmt.Sprintf("%s could not check %s (%v); CI still checks it", check.f.name, check.file, check.err))
		case check.formatted != blobs[check.file]:
			problems = append(problems, fmt.Sprintf("%s is not %s-formatted as committed; run `%s` and commit the result", check.file, check.f.name, check.f.fix))
		}
	}
	return problems, warnings, nil
}

func committedBlobs(root, revision string, files []string) (map[string]string, error) {
	blobs := make(map[string]string, len(files))
	if len(files) == 0 {
		return blobs, nil
	}
	var request strings.Builder
	for _, file := range files {
		request.WriteString(revision + ":" + file + "\n")
	}
	command := exec.Command("git", "-C", root, "cat-file", "--batch")
	command.Stdin = strings.NewReader(request.String())
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git cat-file --batch: %w", err)
	}
	reader := bufio.NewReader(bytes.NewReader(output))
	for _, file := range files {
		header, err := reader.ReadString('\n')
		fields := strings.Fields(header)
		if err != nil || len(fields) != 3 || fields[1] != "blob" {
			return nil, fmt.Errorf("git cat-file --batch: %s:%s: %s", revision, file, strings.TrimSpace(header))
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("git cat-file --batch: %s:%s: size %q", revision, file, fields[2])
		}
		content := make([]byte, size+1)
		if _, err := io.ReadFull(reader, content); err != nil {
			return nil, fmt.Errorf("git cat-file --batch: %s:%s: %w", revision, file, err)
		}
		blobs[file] = string(content[:size])
	}
	return blobs, nil
}

func committedCommentProblems(root, rangeSpec string) ([]string, error) {
	names, err := gitOutput(root, "diff", "--name-only", "--diff-filter=ACMR", rangeSpec)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, file := range lines(names) {
		if _, ok := commentPrefixesFor(path.Base(file)); ok && commentBudgetCovers(file) {
			files = append(files, file)
		}
	}
	blobs, err := committedBlobs(root, rangeEnd(rangeSpec), files)
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, file := range files {
		prefixes, _ := commentPrefixesFor(path.Base(file))
		if found := commentViolationsIn([]byte(blobs[file]), prefixes); len(found) > 0 {
			problems = append(problems, commentBudgetProblem(file, len(found)))
		}
	}
	return problems, nil
}

func formatterFor(root, file string) (formatter, bool) {
	switch {
	case strings.HasSuffix(file, extGo) && (strings.HasPrefix(file, dirAppsPlatform+"/") || strings.HasPrefix(file, dirAppsSandbox+"/") || strings.HasPrefix(file, dirToolsDevctl+"/")):
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

func (f formatter) availability() error {
	if _, err := exec.LookPath(f.cmd); err != nil {
		return fmt.Errorf("%w: %w", errFormatterUnavailable, err)
	}
	if f.requires != "" && !fileExists(f.requires) {
		return fmt.Errorf("%w: %s is missing, run task bootstrap", errFormatterUnavailable, f.requires)
	}
	return nil
}

func (f formatter) format(root, source string) (string, error) {
	if err := f.availability(); err != nil {
		return "", err
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
