package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

const (
	domainMemoryPluginDir   = ".claude/skills/domain-memory"
	domainMemoryManifest    = domainMemoryPluginDir + "/.claude-plugin/plugin.json"
	domainMemoryReleaseFile = "tools/devctl/domain-memory-release.json"
	cmdDomainMemoryRelease  = "domain-memory-release"
	domainMemoryReleaseCmd  = "go -C tools/devctl run . " + cmdDomainMemoryRelease
)

type domainMemoryRelease struct {
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

func isDomainMemoryDistributed(file string) bool {
	if strings.HasPrefix(file, domainMemoryPluginDir+"/evals/") {
		return false
	}
	base := path.Base(file)
	return !strings.HasPrefix(base, "test_") || !strings.HasSuffix(base, ".py")
}

func domainMemoryDistributedFiles(root string) ([]string, error) {
	listed, err := gitOutput(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", domainMemoryPluginDir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, file := range strings.Split(listed, "\x00") {
		if file != "" && isDomainMemoryDistributed(file) {
			files = append(files, file)
		}
	}
	sort.Strings(files)
	return files, nil
}

func domainMemoryDigest(root string, files []string) (string, error) {
	hash := sha256.New()
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			return "", err
		}
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		fmt.Fprintf(hash, "%s\x00%d\x00", file, len(data))
		hash.Write(data)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func currentDomainMemoryRelease(root string) (domainMemoryRelease, error) {
	var manifest struct {
		Version string `json:"version"`
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(domainMemoryManifest)))
	if err != nil {
		return domainMemoryRelease{}, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Version == "" {
		return domainMemoryRelease{}, fmt.Errorf("%s has no readable version", domainMemoryManifest)
	}
	files, err := domainMemoryDistributedFiles(root)
	if err != nil {
		return domainMemoryRelease{}, err
	}
	digest, err := domainMemoryDigest(root, files)
	if err != nil {
		return domainMemoryRelease{}, err
	}
	return domainMemoryRelease{Version: manifest.Version, Digest: digest}, nil
}

func recordedDomainMemoryRelease(root string) (domainMemoryRelease, error) {
	var recorded domainMemoryRelease
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(domainMemoryReleaseFile)))
	if err != nil {
		return recorded, err
	}
	if err := json.Unmarshal(data, &recorded); err != nil {
		return recorded, fmt.Errorf("%s: %w", domainMemoryReleaseFile, err)
	}
	return recorded, nil
}

func contentChangedWithoutBump(current, recorded domainMemoryRelease) bool {
	return current.Digest != recorded.Digest && current.Version == recorded.Version
}

func bumpMessage(version string) string {
	return fmt.Sprintf("the Domain Memory plugin changed but %s still says version %s; bump the version so installed copies "+
		"see the change, then run %s", domainMemoryManifest, version, domainMemoryReleaseCmd)
}

func domainMemoryReleaseProblems(root string) []string {
	if !fileExists(filepath.Join(root, filepath.FromSlash(domainMemoryManifest))) {
		return nil
	}
	current, err := currentDomainMemoryRelease(root)
	if err != nil {
		return []string{fmt.Sprintf("Domain Memory release: %v", err)}
	}
	recorded, err := recordedDomainMemoryRelease(root)
	if errors.Is(err, os.ErrNotExist) {
		return []string{fmt.Sprintf("Domain Memory release: %s is missing; run %s", domainMemoryReleaseFile, domainMemoryReleaseCmd)}
	}
	if err != nil {
		return []string{fmt.Sprintf("Domain Memory release: %v", err)}
	}
	if current == recorded {
		return nil
	}
	if contentChangedWithoutBump(current, recorded) {
		return []string{"Domain Memory release: " + bumpMessage(current.Version)}
	}
	return []string{fmt.Sprintf("Domain Memory release: %s records %s but the plugin is %s; run %s",
		domainMemoryReleaseFile, recorded.Version, current.Version, domainMemoryReleaseCmd)}
}

func recordDomainMemoryRelease(root string, out io.Writer) error {
	current, err := currentDomainMemoryRelease(root)
	if err != nil {
		return err
	}
	recorded, err := recordedDomainMemoryRelease(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && contentChangedWithoutBump(current, recorded) {
		return errors.New(bumpMessage(current.Version))
	}
	data, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(domainMemoryReleaseFile)), append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "recorded Domain Memory %s %s\n", current.Version, current.Digest)
	return nil
}
