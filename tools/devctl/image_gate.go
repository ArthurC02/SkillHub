package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const runtimeImageDir = "infra/images/runtime-agent-sdk"

var (
	digestPinnedImage    = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	imageVersionDiffLine = regexp.MustCompile(`(?m)^[+-]ARG\s+IMAGE_VERSION\s*=`)
)

func imageGate(root string, args []string, out io.Writer) error {
	if len(args) > 0 {
		return errors.New("usage: devctl image-gate")
	}
	problems := digestPinProblems(root)
	problems = append(problems, imageVersionProblems(root)...)
	bump, err := imageBumpProblems(root)
	if err != nil {
		return err
	}
	problems = append(problems, bump...)
	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintln(out, "FAIL", problem)
		}
		return errors.New("runtime image source gates failed")
	}
	fmt.Fprintln(out, "runtime image source gates passed")
	return nil
}

func digestPinProblems(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runtimeDockerfile)))
	if err != nil {
		return []string{fmt.Sprintf("I-02: %v", err)}
	}
	return unpinnedBaseImages(string(data))
}

func unpinnedBaseImages(dockerfile string) []string {
	unpinned, sawFrom := unpinnedFromImages(dockerfile)
	var problems []string
	for _, image := range unpinned {
		problems = append(problems, fmt.Sprintf("I-02: %s has `FROM %s`, which is not pinned by digest", runtimeDockerfile, image))
	}
	if !sawFrom {
		problems = append(problems, fmt.Sprintf("I-02: %s has no FROM instruction", runtimeDockerfile))
	}
	return problems
}

func unpinnedFromImages(dockerfile string) (unpinned []string, sawFrom bool) {
	stages := map[string]bool{}
	for _, instruction := range dockerInstructions(dockerfile) {
		fields := strings.Fields(instruction)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		args := withoutFlags(fields[1:])
		if len(args) == 0 {
			continue
		}
		sawFrom = true
		image := args[0]
		if !stages[strings.ToLower(image)] && !digestPinnedImage.MatchString(image) {
			unpinned = append(unpinned, image)
		}
		if len(args) >= 3 && strings.EqualFold(args[1], "AS") {
			stages[strings.ToLower(args[2])] = true
		}
	}
	return unpinned, sawFrom
}

func withoutFlags(args []string) []string {
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		args = args[1:]
	}
	return args
}

func dockerInstructions(dockerfile string) []string {
	var instructions []string
	var current strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(dockerfile, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if continued, ok := strings.CutSuffix(trimmed, `\`); ok {
			current.WriteString(continued)
			current.WriteString(" ")
			continue
		}
		current.WriteString(trimmed)
		instructions = append(instructions, current.String())
		current.Reset()
	}
	if current.Len() > 0 {
		instructions = append(instructions, current.String())
	}
	return instructions
}

type imageInputs struct {
	paths      []string
	everything bool
}

// A source this parser cannot pin to a path (a glob, the whole context, the
// JSON form) makes every file count as an input, so a miss fails closed.
func runtimeImageInputs(dockerfile string) imageInputs {
	var inputs imageInputs
	for _, instruction := range dockerInstructions(dockerfile) {
		fields := strings.Fields(instruction)
		if len(fields) == 0 || (!strings.EqualFold(fields[0], "COPY") && !strings.EqualFold(fields[0], "ADD")) {
			continue
		}
		if copiesFromAnotherStage(fields[1:]) {
			continue
		}
		args := withoutFlags(fields[1:])
		if len(args) > 0 && strings.HasPrefix(args[0], "[") {
			inputs.everything = true
			continue
		}
		if len(args) < 2 {
			continue
		}
		for _, source := range args[:len(args)-1] {
			cleaned := strings.TrimPrefix(path.Clean(source), "/")
			switch {
			case strings.Contains(source, "://"):
			case cleaned == "" || cleaned == "." || strings.ContainsAny(cleaned, "*?["):
				inputs.everything = true
			default:
				inputs.paths = append(inputs.paths, cleaned)
			}
		}
	}
	return inputs
}

func copiesFromAnotherStage(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "--") {
			return false
		}
		if strings.HasPrefix(arg, "--from=") {
			return true
		}
	}
	return false
}

func (inputs imageInputs) include(file string) bool {
	if file == "Dockerfile" || inputs.everything {
		return true
	}
	for _, source := range inputs.paths {
		if file == source || strings.HasPrefix(file, source+"/") {
			return true
		}
	}
	return false
}

func imageBumpProblems(root string) ([]string, error) {
	dockerfile, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(runtimeDockerfile)))
	if err != nil {
		return nil, err
	}
	version := declaredImageVersion(string(dockerfile))
	if version == "" {
		return nil, nil
	}
	published, err := commitThatDeclared(root, version)
	if err != nil || published == "" {
		return nil, err
	}
	changed, err := inputsChangedSince(root, published, string(dockerfile))
	if err != nil || len(changed) == 0 {
		return nil, err
	}
	return []string{fmt.Sprintf(
		"I-05: %s no longer matches what was published as IMAGE_VERSION=%s (%s differ, comparing against %s); "+
			"bump the version in %s and add that version's section to %s, or restore the content that version was built from",
		runtimeImageDir, version, strings.Join(changed, ", "), published[:12], runtimeDockerfile, runtimeUpgrades)}, nil
}

func declaredImageVersion(dockerfile string) string {
	matches := imageVersionArg.FindAllStringSubmatch(dockerfile, -1)
	if len(matches) != 1 {
		return ""
	}
	return matches[0][1]
}

// Walks the Dockerfile's history newest first and stops at the first commit
// declaring a different version; the commit before it is where the current
// version was introduced, which is the content that was published under it.
func commitThatDeclared(root, version string) (string, error) {
	history, err := gitOutput(root, "log", "--format=%H", "--", runtimeDockerfile)
	if err != nil {
		return "", err
	}
	introduced := ""
	for _, commit := range lines(history) {
		dockerfile, err := gitOutput(root, "show", commit+":"+runtimeDockerfile)
		if err != nil {
			return "", err
		}
		if declaredImageVersion(dockerfile) != version {
			break
		}
		introduced = commit
	}
	return introduced, nil
}

func inputsChangedSince(root, commit, dockerfile string) ([]string, error) {
	inputs := runtimeImageInputs(dockerfile)
	names, err := gitOutput(root, "diff", "--name-only", commit, "HEAD", "--", runtimeImageDir)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, name := range lines(names) {
		if file := strings.TrimPrefix(name, runtimeImageDir+"/"); inputs.include(file) {
			changed = append(changed, file)
		}
	}
	return changed, nil
}

func rangeEnd(rangeSpec string) string {
	i := strings.LastIndex(rangeSpec, "..")
	if i < 0 {
		return rangeSpec
	}
	if end := rangeSpec[i+2:]; end != "" {
		return end
	}
	return "HEAD"
}

func gitOutput(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

func lines(text string) []string {
	var result []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}
