package main

import (
	"errors"
	"fmt"
	"io"
	"os"
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
	head, err := gitOutput(dir, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(head)
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
		if _, err := gitOutput(root, "init", "-q", domainMemoryInstallDir); err != nil {
			return err
		}
	}
	if _, err := gitOutput(dir, "fetch", "-q", "--depth", "1", repository, pin); err != nil {
		return err
	}
	_, err := gitOutput(dir, "-c", "core.autocrlf=false", "checkout", "-q", "--force", "--detach", "FETCH_HEAD")
	return err
}
