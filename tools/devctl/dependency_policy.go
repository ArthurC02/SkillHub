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
)

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
	var problems []string
	updated := map[string]bool{}
	composeFiles, ciFiles := map[string]string{}, map[string]string{}
	for _, file := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		dir, base := path.Dir(file), path.Base(file)
		switch {
		case base == "package-lock.json":
			updated[dir] = true
			npmrc, err := read(path.Join(dir, ".npmrc"))
			if err != nil || !npmrcIgnoresScripts(npmrc) {
				problems = append(problems, fmt.Sprintf("%s/.npmrc must set ignore-scripts=true", dir))
			}
		case base == "uv.lock":
			updated[dir] = true
			pyproject, err := read(path.Join(dir, "pyproject.toml"))
			if err != nil || !uvCooldown.MatchString(pyproject) {
				problems = append(problems, fmt.Sprintf("%s/pyproject.toml must set [tool.uv] exclude-newer", dir))
			}
		case base == "go.mod":
			updated[dir] = true
		case strings.HasPrefix(base, "Dockerfile"):
			updated[dir] = true
			content, err := read(file)
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}
			problems = append(problems, dockerfilePinProblems(file, content)...)
		case strings.HasPrefix(file, "infra/compose/") && isYAML(base):
			updated[dir] = true
			content, err := read(file)
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}
			composeFiles[file] = content
			problems = append(problems, composeImageProblems(file, content)...)
		case (strings.HasPrefix(file, ".github/workflows/") || strings.HasPrefix(file, ".github/actions/")) && isYAML(base):
			if strings.HasPrefix(file, ".github/actions/") {
				updated[dir] = true
			}
			content, err := read(file)
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}
			ciFiles[file] = content
			problems = append(problems, workflowPinProblems(file, content)...)
		case strings.HasPrefix(file, "tools/ci/") && strings.HasSuffix(base, ".sh"):
			content, err := read(file)
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}
			ciFiles[file] = content
		}
	}
	for dir := range updated {
		if !dependabotCovers(string(dependabot), dir) {
			problems = append(problems, fmt.Sprintf(".github/dependabot.yml does not list /%s", dir))
		}
	}
	problems = append(problems, versionAgreementProblems(versionsThatMoveTogether, read)...)
	return append(problems, composeAndWorkflowImageDrift(composeFiles, ciFiles)...)
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
