package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	cmdDomainMemoryInstall = "domain-memory-install"
	domainMemoryReleases   = "https://github.com/ArthurC02/Domain-Memory-PlugIn/releases/download"
	domainMemoryInstallDir = ".domain-memory"
	domainMemorySumsFile   = "SHA256SUMS"
	domainMemoryInstallCmd = "go -C tools/devctl run . " + cmdDomainMemoryInstall

	executableMode os.FileMode = 0o755
	dataFileMode   os.FileMode = 0o644
)

type domainMemoryRelease struct {
	tag, sums string
}

func domainMemoryPin(root string) (domainMemoryRelease, error) {
	plugins, err := parseManifestSection(filepath.Join(root, filepath.FromSlash(toolchainYAML)), "plugins")
	if err != nil {
		return domainMemoryRelease{}, err
	}
	pin := domainMemoryRelease{tag: plugins["domain_memory"], sums: plugins["domain_memory_sha256sums"]}
	if pin.tag == "" || pin.sums == "" {
		return domainMemoryRelease{}, fmt.Errorf("%s must pin plugins.domain_memory (a release tag) and plugins.domain_memory_sha256sums (the SHA-256 of its %s)", toolchainYAML, domainMemorySumsFile)
	}
	return pin, nil
}

func domainMemoryAsset(goos, goarch string) (string, error) {
	switch {
	case goos == "linux" && goarch == "amd64":
		return "domain-memory-linux-x86_64", nil
	case goos == "darwin":
		return "domain-memory-macos-universal", nil
	case goos == "windows" && goarch == "amd64":
		return "domain-memory-windows-x86_64.exe", nil
	}
	return "", fmt.Errorf("the Domain Memory releases carry no executable for %s/%s; build one from a clone of the plugin with cargo build --release --locked --manifest-path engine/Cargo.toml", goos, goarch)
}

func domainMemoryExecutable(root, goos string) string {
	name := "domain-memory"
	if goos == "windows" {
		name += ".exe"
	}
	return filepath.Join(root, domainMemoryInstallDir, name)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sumFor(sums []byte, asset string) string {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == asset {
			return fields[0]
		}
	}
	return ""
}

func installedDomainMemoryProblem(root string, pin domainMemoryRelease, asset string) string {
	sums, err := os.ReadFile(filepath.Join(root, domainMemoryInstallDir, domainMemorySumsFile))
	if err != nil {
		return "nothing is installed"
	}
	if sha256Hex(sums) != pin.sums {
		return "the installed release is not the pinned one"
	}
	executable, err := os.ReadFile(domainMemoryExecutable(root, runtime.GOOS))
	if err != nil {
		return "the executable is missing"
	}
	if sha256Hex(executable) != sumFor(sums, asset) {
		return "the executable does not match its " + domainMemorySumsFile + " entry"
	}
	return ""
}

func download(client *http.Client, url string) ([]byte, error) {
	response, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, response.Status)
	}
	return io.ReadAll(response.Body)
}

func installDomainMemory(root, releases string, out io.Writer) error {
	pin, err := domainMemoryPin(root)
	if err != nil {
		return err
	}
	asset, err := domainMemoryAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if installedDomainMemoryProblem(root, pin, asset) != "" {
		if err := downloadDomainMemory(root, releases, pin, asset); err != nil {
			return err
		}
	}
	if err := pointHooksAtDomainMemory(root); err != nil {
		return err
	}
	fmt.Fprintf(out, "Domain Memory %s is installed at %s\n", pin.tag, domainMemoryExecutable(root, runtime.GOOS))
	return nil
}

func downloadDomainMemory(root, releases string, pin domainMemoryRelease, asset string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	base := releases + "/" + pin.tag
	sums, err := download(client, base+"/"+domainMemorySumsFile)
	if err != nil {
		return err
	}
	if got := sha256Hex(sums); got != pin.sums {
		return fmt.Errorf("%s of Domain Memory %s has SHA-256 %s, but %s pins %s; nothing was installed", domainMemorySumsFile, pin.tag, got, toolchainYAML, pin.sums)
	}
	executable, err := download(client, base+"/"+asset)
	if err != nil {
		return err
	}
	if got, want := sha256Hex(executable), sumFor(sums, asset); want == "" || got != want {
		return fmt.Errorf("%s of Domain Memory %s does not match its %s entry; nothing was installed", asset, pin.tag, domainMemorySumsFile)
	}
	dir := filepath.Join(root, domainMemoryInstallDir)
	if err := os.MkdirAll(dir, executableMode); err != nil {
		return err
	}
	if err := writeAtomically(domainMemoryExecutable(root, runtime.GOOS), executable, executableMode); err != nil {
		return err
	}
	return writeAtomically(filepath.Join(dir, domainMemorySumsFile), sums, dataFileMode)
}

func writeAtomically(path string, data []byte, mode os.FileMode) error {
	staged := path + ".download"
	if err := os.WriteFile(staged, data, mode); err != nil {
		return err
	}
	return os.Rename(staged, path)
}

func pointHooksAtDomainMemory(root string) error {
	settings := [][2]string{
		{"domain-memory.executable", domainMemoryInstallDir + "/" + filepath.Base(domainMemoryExecutable(root, runtime.GOOS))},
		{"domain-memory.install-hint", domainMemoryInstallCmd},
	}
	for _, setting := range settings {
		cmd := exec.Command("git", "-C", root, "config", "--local", setting[0], setting[1])
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git config %s: %s", setting[0], strings.TrimSpace(string(output)))
		}
	}
	return nil
}
