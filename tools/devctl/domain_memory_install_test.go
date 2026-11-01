package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

type fakeRelease struct {
	server   *httptest.Server
	requests *atomic.Int32
	sums     string
}

func publishRelease(t *testing.T, tag string, executable, served []byte) fakeRelease {
	t.Helper()
	asset, err := domainMemoryAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Skip(err)
	}
	sums := []byte(sha256Hex(executable) + "  " + asset + "\n" + strings.Repeat("0", 64) + "  install.sh\n")
	files := map[string][]byte{"/" + tag + "/" + domainMemorySumsFile: sums, "/" + tag + "/" + asset: served}
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return fakeRelease{server: server, requests: requests, sums: sha256Hex(sums)}
}

func consumerPinning(t *testing.T, tag, sums string) string {
	t.Helper()
	repo := newGitRepo(t)
	writeTestFile(t, repo.root, "tools/toolchain.yaml", "schema_version: 1\n\nplugins:\n  domain_memory: \""+tag+"\"\n  domain_memory_sha256sums: \""+sums+"\"\n")
	return repo.root
}

func gitConfig(t *testing.T, root, key string) string {
	t.Helper()
	value, err := gitOutput(root, "config", "--local", key)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(value)
}

func TestInstallPutsThePinnedExecutableInTheInstallDirectory(t *testing.T) {
	t.Parallel()
	release := publishRelease(t, "v1.0.0", []byte("executable v1"), []byte("executable v1"))
	root := consumerPinning(t, "v1.0.0", release.sums)
	if err := installDomainMemory(root, release.server.URL, io.Discard); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(domainMemoryExecutable(root, runtime.GOOS)); err != nil || string(got) != "executable v1" {
		t.Fatalf("installed executable = %q, %v; want the release's", got, err)
	}
}

func TestInstallPointsTheHooksAtTheInstalledExecutable(t *testing.T) {
	t.Parallel()
	release := publishRelease(t, "v1.0.0", []byte("executable v1"), []byte("executable v1"))
	root := consumerPinning(t, "v1.0.0", release.sums)
	if err := installDomainMemory(root, release.server.URL, io.Discard); err != nil {
		t.Fatal(err)
	}
	want := ".domain-memory/domain-memory"
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got := gitConfig(t, root, "domain-memory.executable"); got != want {
		t.Errorf("domain-memory.executable = %q, want %q", got, want)
	}
	if got := gitConfig(t, root, "domain-memory.install-hint"); got != "go -C tools/devctl run . domain-memory-install" {
		t.Errorf("domain-memory.install-hint = %q, want the install command", got)
	}
}

func TestInstallAtThePinnedReleaseDownloadsNothing(t *testing.T) {
	t.Parallel()
	release := publishRelease(t, "v1.0.0", []byte("executable v1"), []byte("executable v1"))
	root := consumerPinning(t, "v1.0.0", release.sums)
	if err := installDomainMemory(root, release.server.URL, io.Discard); err != nil {
		t.Fatal(err)
	}
	before := release.requests.Load()
	if err := installDomainMemory(root, release.server.URL, io.Discard); err != nil {
		t.Fatal(err)
	}
	if after := release.requests.Load(); after != before {
		t.Fatalf("a second install at the pin made %d requests, want none", after-before)
	}
}

func TestInstallRefusesSumsThatAreNotThePinnedOnes(t *testing.T) {
	t.Parallel()
	release := publishRelease(t, "v1.0.0", []byte("executable v1"), []byte("executable v1"))
	root := consumerPinning(t, "v1.0.0", strings.Repeat("ab", 32))
	err := installDomainMemory(root, release.server.URL, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "but tools/toolchain.yaml pins "+strings.Repeat("ab", 32)+"; nothing was installed") {
		t.Fatalf("err = %v, want the pinned digest named", err)
	}
	if _, err := os.Stat(domainMemoryExecutable(root, runtime.GOOS)); !os.IsNotExist(err) {
		t.Fatalf("an executable was installed despite the refusal: %v", err)
	}
}

func TestInstallRefusesAnExecutableThatDoesNotMatchItsSum(t *testing.T) {
	t.Parallel()
	release := publishRelease(t, "v1.0.0", []byte("executable v1"), []byte("tampered"))
	root := consumerPinning(t, "v1.0.0", release.sums)
	err := installDomainMemory(root, release.server.URL, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "does not match its SHA256SUMS entry; nothing was installed") {
		t.Fatalf("err = %v, want the mismatch named", err)
	}
	if _, err := os.Stat(domainMemoryExecutable(root, runtime.GOOS)); !os.IsNotExist(err) {
		t.Fatalf("an executable was installed despite the refusal: %v", err)
	}
}

func TestInstallNamesAReleaseThatIsNotPublished(t *testing.T) {
	t.Parallel()
	release := publishRelease(t, "v1.0.0", []byte("executable v1"), []byte("executable v1"))
	root := consumerPinning(t, "v9.9.9", release.sums)
	err := installDomainMemory(root, release.server.URL, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "/v9.9.9/SHA256SUMS: 404") {
		t.Fatalf("err = %v, want the missing release named", err)
	}
}

func TestInstallRefusesAToolchainWithoutAPin(t *testing.T) {
	t.Parallel()
	root := newGitRepo(t).root
	writeTestFile(t, root, "tools/toolchain.yaml", "schema_version: 1\n\nplugins:\n  domain_memory: \"v1.0.0\"\n")
	err := installDomainMemory(root, "http://127.0.0.1:1", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "plugins.domain_memory_sha256sums") {
		t.Fatalf("err = %v, want the missing digest pin named", err)
	}
}

func TestTheToolRefusesToRunAnInstallThatIsNotThePin(t *testing.T) {
	t.Parallel()
	release := publishRelease(t, "v1.0.0", []byte("executable v1"), []byte("executable v1"))
	root := consumerPinning(t, "v1.0.0", release.sums)
	if err := installDomainMemory(root, release.server.URL, io.Discard); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "tools/toolchain.yaml", "plugins:\n  domain_memory: \"v2.0.0\"\n  domain_memory_sha256sums: \""+strings.Repeat("cd", 32)+"\"\n")
	_, err := domainMemoryTool(root, "validate")
	if err == nil || !strings.Contains(err.Error(), "v2.0.0 is not installed in .domain-memory (the installed release is not the pinned one); run "+domainMemoryInstallCmd) {
		t.Fatalf("err = %v, want the install command named", err)
	}
}
