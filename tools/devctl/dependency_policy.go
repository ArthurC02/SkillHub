package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	composeImageLine = regexp.MustCompile(`(?m)^[ \t]*image:[ \t]*["']?([^"'\s#]+)`)
	usesLine         = regexp.MustCompile(`(?m)^[ \t]*(?:-[ \t]+)?uses:[ \t]*["']?([^"'\s#]+)["']?(.*)$`)
	shaPinnedAction  = regexp.MustCompile(`@[0-9a-f]{40}$`)
	versionComment   = regexp.MustCompile(`^\s+#\s*v\d`)
	uvCooldown       = regexp.MustCompile(`(?m)^exclude-newer\s*=`)
	pinnedImageRef   = regexp.MustCompile(`[a-z0-9][a-z0-9./_-]*:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}`)
	deployImageVar   = regexp.MustCompile(`^\$\{([A-Z][A-Z0-9_]*)`)
)

const deployPreflightSuffix = "/bin/skillhub-preflight"

func dependencyPolicyProblems(root string) []string {
	out, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		return []string{fmt.Sprintf("git ls-files: %v", err)}
	}
	dependabot, err := os.ReadFile(filepath.Join(root, ".github", "dependabot.yml"))
	if err != nil {
		return []string{fmt.Sprintf(".github/dependabot.yml: %v", err)}
	}
	read := func(file string) (string, error) {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		return string(data), err
	}
	survey := &dependencyFileSurvey{
		read:         read,
		updated:      map[string]bool{},
		composeFiles: map[string]string{},
		ciFiles:      map[string]string{},
	}
	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		survey.record(file)
	}
	problems := survey.problems
	for _, file := range sortedKeys(survey.composeFiles) {
		problems = append(problems, deployComposeImageProblems(file, survey.composeFiles[file], survey.preflights.String())...)
	}
	for dir := range survey.updated {
		if !dependabotCovers(string(dependabot), dir) {
			problems = append(problems, fmt.Sprintf(".github/dependabot.yml does not list /%s", dir))
		}
	}
	problems = append(problems, versionAgreementProblems(versionsThatMoveTogether, read)...)
	return append(problems, composeAndWorkflowImageDrift(survey.composeFiles, survey.ciFiles)...)
}

type dependencyFileSurvey struct {
	read         func(string) (string, error)
	problems     []string
	updated      map[string]bool
	composeFiles map[string]string
	ciFiles      map[string]string
	preflights   strings.Builder
}

func (s *dependencyFileSurvey) record(file string) {
	dir, base := path.Dir(file), path.Base(file)
	switch {
	case base == "package-lock.json":
		s.recordPackageLock(dir)
	case base == "uv.lock":
		s.recordUVLock(dir)
	case base == "go.mod":
		s.updated[dir] = true
	case strings.HasPrefix(base, "Dockerfile"):
		s.recordDockerfile(dir, file)
	case strings.HasPrefix(file, "infra/compose/") && isYAML(base):
		s.recordComposeFile(dir, file)
	case strings.HasPrefix(file, "infra/deploy/") && strings.HasSuffix(file, deployPreflightSuffix):
		s.recordDeployPreflight(file)
	case (strings.HasPrefix(file, ".github/workflows/") || strings.HasPrefix(file, ".github/actions/")) && isYAML(base):
		s.recordWorkflowFile(dir, file)
	case strings.HasPrefix(file, "tools/ci/") && strings.HasSuffix(base, ".sh"):
		s.recordCIScript(file)
	}
}

func (s *dependencyFileSurvey) recordPackageLock(dir string) {
	s.updated[dir] = true
	s.require(path.Join(dir, ".npmrc"), npmrcIgnoresScripts,
		fmt.Sprintf("%s/.npmrc must set ignore-scripts=true", dir))
}

func (s *dependencyFileSurvey) recordUVLock(dir string) {
	s.updated[dir] = true
	s.require(path.Join(dir, "pyproject.toml"), uvCooldown.MatchString,
		fmt.Sprintf("%s/pyproject.toml must set [tool.uv] exclude-newer", dir))
}

func (s *dependencyFileSurvey) recordDockerfile(dir, file string) {
	s.updated[dir] = true
	if content, ok := s.readTracked(file); ok {
		s.problems = append(s.problems, dockerfilePinProblems(file, content)...)
	}
}

func (s *dependencyFileSurvey) recordComposeFile(dir, file string) {
	s.updated[dir] = true
	if content, ok := s.readTracked(file); ok {
		s.composeFiles[file] = content
	}
}

func (s *dependencyFileSurvey) recordDeployPreflight(file string) {
	if content, ok := s.readTracked(file); ok {
		s.preflights.WriteString(content)
	}
}

func (s *dependencyFileSurvey) recordWorkflowFile(dir, file string) {
	if strings.HasPrefix(file, ".github/actions/") {
		s.updated[dir] = true
	}
	if content, ok := s.readTracked(file); ok {
		s.ciFiles[file] = content
		s.problems = append(s.problems, workflowPinProblems(file, content)...)
	}
}

func (s *dependencyFileSurvey) recordCIScript(file string) {
	if content, ok := s.readTracked(file); ok {
		s.ciFiles[file] = content
	}
}

func (s *dependencyFileSurvey) require(file string, holds func(string) bool, problem string) {
	content, err := s.read(file)
	if err != nil || !holds(content) {
		s.problems = append(s.problems, problem)
	}
}

func (s *dependencyFileSurvey) readTracked(file string) (string, bool) {
	content, err := s.read(file)
	if err != nil {
		s.problems = append(s.problems, err.Error())
		return "", false
	}
	return content, true
}

func isYAML(base string) bool {
	return strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")
}

func npmrcIgnoresScripts(npmrc string) bool {
	for _, line := range strings.Split(npmrc, "\n") {
		if strings.ReplaceAll(strings.TrimSpace(line), " ", "") == "ignore-scripts=true" {
			return true
		}
	}
	return false
}

func dockerfilePinProblems(file, content string) []string {
	var problems []string
	unpinned, _ := unpinnedFromImages(content)
	for _, image := range unpinned {
		problems = append(problems, fmt.Sprintf("%s: FROM %s is not pinned by digest", file, image))
	}
	return problems
}

func composeImageProblems(file, content string) []string {
	var problems []string
	for _, match := range composeImageLine.FindAllStringSubmatch(content, -1) {
		if !digestPinnedImage.MatchString(match[1]) {
			problems = append(problems, fmt.Sprintf("%s: image %s is not pinned by digest", file, match[1]))
		}
	}
	return problems
}

func deployComposeImageProblems(file, content, preflights string) []string {
	var problems []string
	for _, match := range composeImageLine.FindAllStringSubmatch(content, -1) {
		variable := deployImageVar.FindStringSubmatch(match[1])
		switch {
		case variable == nil:
			if !digestPinnedImage.MatchString(match[1]) {
				problems = append(problems, fmt.Sprintf("%s: image %s is not pinned by digest", file, match[1]))
			}
		case !regexp.MustCompile(`\b` + variable[1] + `\b`).MatchString(preflights):
			problems = append(problems, fmt.Sprintf(
				"%s: image ${%s} is chosen at deploy time, but no infra/deploy/*%s refuses a value that is not pinned by digest",
				file, variable[1], deployPreflightSuffix))
		}
	}
	return problems
}

func workflowPinProblems(file, content string) []string {
	problems := composeImageProblems(file, content)
	for _, match := range usesLine.FindAllStringSubmatch(content, -1) {
		ref, rest := match[1], match[2]
		switch {
		case strings.HasPrefix(ref, "./"):
		case strings.HasPrefix(ref, "docker://"):
			if !digestPinnedImage.MatchString(ref) {
				problems = append(problems, fmt.Sprintf("%s: uses %s is not pinned by digest", file, ref))
			}
		case !shaPinnedAction.MatchString(ref):
			problems = append(problems, fmt.Sprintf("%s: uses %s is not pinned to a commit SHA", file, ref))
		case !versionComment.MatchString(rest):
			problems = append(problems, fmt.Sprintf("%s: uses %s has no # vX version comment after the SHA", file, ref))
		}
	}
	return problems
}

func composeAndWorkflowImageDrift(composeFiles, ciFiles map[string]string) []string {
	compose := map[string]string{}
	for _, content := range composeFiles {
		for _, ref := range pinnedImageRef.FindAllString(content, -1) {
			compose[imageRepository(ref)] = canonicalImage(ref)
		}
	}
	var problems []string
	for file, content := range ciFiles {
		for _, ref := range pinnedImageRef.FindAllString(content, -1) {
			if want, shared := compose[imageRepository(ref)]; shared && canonicalImage(ref) != want {
				problems = append(problems, fmt.Sprintf("%s: %s differs from infra/compose's %s; bump both together", file, ref, want))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func canonicalImage(ref string) string {
	return strings.TrimPrefix(strings.TrimPrefix(ref, "docker.io/"), "library/")
}

func imageRepository(ref string) string {
	name, _, _ := strings.Cut(canonicalImage(ref), "@")
	return name[:strings.LastIndex(name, ":")]
}

func dependabotCovers(config, dir string) bool {
	pattern := regexp.MustCompile(`(?m)(?:-|directory:)\s*["']?/` + regexp.QuoteMeta(dir) + `["']?\s*$`)
	return pattern.MatchString(config)
}
