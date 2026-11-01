package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	cmdDomainMemoryInstall = "domain-memory-install"
	domainMemoryRepository = "https://github.com/ArthurC02/Domain-Memory-PlugIn.git"
	domainMemoryInstallDir = ".domain-memory"
	domainMemoryPinKey     = "domain_memory"
	domainMemoryInstallCmd = "go -C tools/devctl run . " + cmdDomainMemoryInstall
)

func domainMemoryPin(root string) (string, error) {
	plugins, err := parseManifestSection(filepath.Join(root, filepath.FromSlash(toolchainYAML)), "plugins")
	if err != nil {
		return "", err
	}
	pin := plugins[domainMemoryPinKey]
	if pin == "" {
		return "", fmt.Errorf("%s pins no plugins.%s commit", toolchainYAML, domainMemoryPinKey)
	}
	return pin, nil
}

func installedDomainMemoryCommit(root string) string {
	dir := filepath.Join(root, domainMemoryInstallDir)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return ""
	}
	head, err := installGit(dir, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(head)
}

func installGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "GIT_DIR=") && !strings.HasPrefix(variable, "GIT_WORK_TREE=") && !strings.HasPrefix(variable, "GIT_INDEX_FILE=") {
			cmd.Env = append(cmd.Env, variable)
		}
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func installDomainMemory(root, repository string, out io.Writer) error {
	pin, err := domainMemoryPin(root)
	if err != nil {
		return err
	}
	if installedDomainMemoryCommit(root) != pin {
		if err := checkOutDomainMemory(root, repository, pin); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "Domain Memory %s is installed in %s\n", pin, domainMemoryInstallDir)
	return nil
}

func checkOutDomainMemory(root, repository, pin string) error {
	dir := filepath.Join(root, domainMemoryInstallDir)
	if _, err := os.Stat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
		if _, err := installGit(root, "init", "-q", domainMemoryInstallDir); err != nil {
			return err
		}
	}
	if _, err := installGit(dir, "fetch", "-q", "--depth", "1", repository, pin); err != nil {
		return err
	}
	_, err := installGit(dir, "-c", "core.autocrlf=false", "checkout", "-q", "--force", "--detach", "FETCH_HEAD")
	return err
}
