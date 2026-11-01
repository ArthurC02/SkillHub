package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pluginSource(t *testing.T, readme string) (gitRepo, string) {
	t.Helper()
	source := newGitRepo(t)
	source.commit(t, map[string]string{"README.md": readme})
	return source, source.head(t)
}

func consumerPinning(t *testing.T, pin string) string {
	t.Helper()
	root := t.TempDir()
	writeTestFile(t, root, "tools/toolchain.yaml", "schema_version: 1\n\ntools:\n  task: \"3.54.0\"\n\nplugins:\n  domain_memory: \""+pin+"\"\n")
	return root
}

func installedReadme(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, domainMemoryInstallDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstallFetchesThePinnedCommitIntoTheInstallDirectory(t *testing.T) {
	t.Parallel()
	source, pin := pluginSource(t, "first\n")
	root := consumerPinning(t, pin)
	if err := installDomainMemory(root, source.root, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := installedDomainMemoryCommit(root); got != pin {
		t.Fatalf("installed %q, want %q", got, pin)
	}
	if got := installedReadme(t, root); got != "first\n" {
		t.Fatalf("installed README = %q, want the pinned commit's", got)
	}
}

func TestInstallAtThePinnedCommitDoesNotFetchAgain(t *testing.T) {
	t.Parallel()
	source, pin := pluginSource(t, "first\n")
	root := consumerPinning(t, pin)
	if err := installDomainMemory(root, source.root, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := installDomainMemory(root, filepath.Join(t.TempDir(), "no-such-repository"), io.Discard); err != nil {
		t.Fatalf("an install already at the pin fetched again: %v", err)
	}
}

func TestInstallMovesAnOlderInstallToANewPin(t *testing.T) {
	t.Parallel()
	source, older := pluginSource(t, "first\n")
	root := consumerPinning(t, older)
	if err := installDomainMemory(root, source.root, io.Discard); err != nil {
		t.Fatal(err)
	}
	source.commit(t, map[string]string{"README.md": "second\n"})
	newer := source.head(t)
	writeTestFile(t, root, "tools/toolchain.yaml", "plugins:\n  domain_memory: \""+newer+"\"\n")
	if err := installDomainMemory(root, source.root, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got := installedReadme(t, root); got != "second\n" {
		t.Fatalf("installed README = %q, want the new pin's", got)
	}
}

func TestInstallRefusesAToolchainWithoutAPin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestFile(t, root, "tools/toolchain.yaml", "schema_version: 1\n\ntools:\n  task: \"3.54.0\"\n")
	err := installDomainMemory(root, t.TempDir(), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "has no plugins entries") {
		t.Fatalf("err = %v, want the missing pin named", err)
	}
}

func TestInstallReportsACommitTheRepositoryLacks(t *testing.T) {
	t.Parallel()
	source, _ := pluginSource(t, "first\n")
	root := consumerPinning(t, strings.Repeat("ab", 20))
	err := installDomainMemory(root, source.root, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "git fetch") {
		t.Fatalf("err = %v, want the failed fetch", err)
	}
	if installedDomainMemoryCommit(root) != "" {
		t.Fatal("a failed fetch left an install behind")
	}
}

func TestTheToolRefusesToRunAnInstallThatIsNotThePin(t *testing.T) {
	t.Parallel()
	source, older := pluginSource(t, "first\n")
	root := consumerPinning(t, older)
	if err := installDomainMemory(root, source.root, io.Discard); err != nil {
		t.Fatal(err)
	}
	source.commit(t, map[string]string{"README.md": "second\n"})
	writeTestFile(t, root, "tools/toolchain.yaml", "plugins:\n  domain_memory: \""+source.head(t)+"\"\n")
	_, err := domainMemoryTool(root, "validate")
	if err == nil || !strings.Contains(err.Error(), "is not installed in .domain-memory; run "+domainMemoryInstallCmd) {
		t.Fatalf("err = %v, want the install command named", err)
	}
}
