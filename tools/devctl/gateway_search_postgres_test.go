package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type searchImageProofCase struct {
	mode, problem string
	builds        bool
	cleanupFails  bool
}

func TestSearchProofOwnsOnlyItsRebuiltImages(t *testing.T) {
	installSearchDockerFixture(t)
	for _, image := range []string{"postgres", "gateway"} {
		t.Run(image, func(t *testing.T) {
			for _, test := range []searchImageProofCase{
				{"healthy", "", true, false},
				{"existing", "already exists", false, false},
				{"list-error", "-tag-check", false, false},
				{"build-empty-error", "-build", true, false},
				{"build-partial-error", "-build", true, false},
				{"inspect-error", "-image-id", true, false},
				{"invalid-identity", "no valid immutable identity", true, false},
				{"cleanup-error", "", true, true},
				{"cleanup-check-error", "", true, true},
			} {
				t.Run(test.mode, func(t *testing.T) {
					test.check(t, image)
				})
			}
		})
	}
}

func installSearchDockerFixture(t *testing.T) {
	t.Helper()
	directory := t.TempDir()
	program := `package main
import ("encoding/json"; "os"; "strings")
func main() {
 args := os.Args[1:]
 file, err := os.OpenFile(os.Getenv("SEARCH_DOCKER_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
 if err != nil { panic(err) }; defer file.Close()
 if err := json.NewEncoder(file).Encode(args); err != nil { panic(err) }
 reference:=args[len(args)-1]
 if args[0]=="build" { reference=args[3] }
 repository:=strings.Split(strings.TrimPrefix(reference,"reference="),":")[0]
 name:=repository[strings.LastIndex(repository,"/")+1:]
 mode, state := os.Getenv("SEARCH_DOCKER_MODE"), os.Getenv("SEARCH_DOCKER_STATE")+"-"+name
 _, present := os.Stat(state)
 switch {
 case len(args)>1 && args[0]=="image" && args[1]=="ls":
  if mode=="list-error" || (mode=="cleanup-check-error" && present==nil) { os.Exit(19) }
  if mode=="existing" || present==nil { os.Stdout.WriteString("sha256:"+strings.Repeat("a",64)+"\n") }
 case len(args)>0 && args[0]=="build":
  if mode=="build-empty-error" { os.Exit(17) }
  if err:=os.WriteFile(state,[]byte("owned"),0600); err!=nil { panic(err) }
  if mode=="build-partial-error" { os.Exit(17) }
 case len(args)>1 && args[0]=="image" && args[1]=="inspect":
  if mode=="inspect-error" { os.Exit(18) }
  value:="sha256:"+strings.Repeat("0",64)
  if mode=="invalid-identity" { value="not-an-image" }
  os.Stdout.WriteString(value+"\n")
 case len(args)>1 && args[0]=="image" && args[1]=="rm":
  if mode=="cleanup-error" { os.Exit(16) }
  if err:=os.Remove(state); err!=nil { panic(err) }
 default: os.Exit(99)
 }
}
`
	file := filepath.Join(directory, "fixture.go")
	if err := os.WriteFile(file, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(directory, "docker")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if output, err := exec.Command("go", "build", "-o", binary, file).CombinedOutput(); err != nil {
		t.Fatalf("build Docker fixture: %v: %s", err, output)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func (test searchImageProofCase) check(t *testing.T, image string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "owned-probe")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	log, state := filepath.Join(root, "calls.jsonl"), filepath.Join(root, "image-state")
	t.Setenv("SEARCH_DOCKER_LOG", log)
	t.Setenv("SEARCH_DOCKER_STATE", state)
	t.Setenv("SEARCH_DOCKER_MODE", test.mode)
	p := &searchProof{root: root, dir: dir, out: io.Discard}
	test.prepare(t, p, image)
	err := p.clean()
	if (err != nil) != test.cleanupFails {
		t.Fatalf("cleanup error = %v, want failure %v", err, test.cleanupFails)
	}
	if test.cleanupFails {
		if p.imageTags[image] != "skillhub-search-proof/"+image+":owned-probe" {
			t.Fatal("failed cleanup lost image ownership")
		}
		t.Setenv("SEARCH_DOCKER_MODE", "healthy")
		if err := p.clean(); err != nil {
			t.Fatal(err)
		}
	}
	if p.imageTags[image] != "" {
		t.Fatal("successful cleanup retained image ownership")
	}
	before, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.clean(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(log)
	if err != nil || string(before) != string(after) {
		t.Fatal("repeated cleanup performed additional Docker operations")
	}
	assertSearchImageCommands(t, root, image, after, test.builds)
	if _, err := os.Stat(state + "-" + image); !os.IsNotExist(err) {
		t.Fatalf("owned image remains after cleanup: %v", err)
	}
}

func TestSearchProofRetainsBothImageOwnersUntilCleanup(t *testing.T) {
	installSearchDockerFixture(t)
	root := t.TempDir()
	state := filepath.Join(root, "image-state")
	t.Setenv("SEARCH_DOCKER_LOG", filepath.Join(root, "calls.jsonl"))
	t.Setenv("SEARCH_DOCKER_STATE", state)
	t.Setenv("SEARCH_DOCKER_MODE", "healthy")
	p := &searchProof{root: root, dir: t.TempDir(), out: io.Discard}
	for _, name := range []string{"gateway", "postgres"} {
		if identity, err := p.prepareImage(name); err != nil || identity != "sha256:"+strings.Repeat("0", 64) {
			t.Fatalf("%s identity = %q, error %v", name, identity, err)
		}
	}
	if len(p.imageTags) != 2 {
		t.Fatalf("owned images = %v, want both gateway and postgres", p.imageTags)
	}
	if err := p.clean(); err != nil {
		t.Fatal(err)
	}
	if len(p.imageTags) != 0 {
		t.Fatalf("cleanup retained ownership: %v", p.imageTags)
	}
	for _, name := range []string{"gateway", "postgres"} {
		if _, err := os.Stat(state + "-" + name); !os.IsNotExist(err) {
			t.Fatalf("%s image remains after cleanup: %v", name, err)
		}
	}
}

func (test searchImageProofCase) prepare(t *testing.T, p *searchProof, image string) {
	t.Helper()
	identity, err := p.prepareImage(image)
	if test.problem == "" {
		if err != nil || identity != "sha256:"+strings.Repeat("0", 64) {
			t.Fatalf("prepared identity = %q, error %v", identity, err)
		}
	} else if err == nil || !strings.Contains(err.Error(), test.problem) {
		t.Fatalf("error = %v, want %q", err, test.problem)
	}
}

func assertSearchImageCommands(t *testing.T, root, image string, calls []byte, wantBuild bool) {
	t.Helper()
	builds, removals := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		var args []string
		if err := json.Unmarshal([]byte(line), &args); err != nil {
			t.Fatal(err)
		}
		var want []string
		switch args[0] {
		case "build":
			builds++
			want = []string{"build", "--progress=plain", "-t", "skillhub-search-proof/" + image + ":owned-probe", filepath.Join(root, "infra/images", image)}
		case "image":
			switch args[1] {
			case "ls":
				want = []string{"image", "ls", "-q", "--filter", "reference=skillhub-search-proof/" + image + ":owned-probe"}
			case "inspect":
				want = []string{"image", "inspect", "--format", "{{.Id}}", "skillhub-search-proof/" + image + ":owned-probe"}
			case "rm":
				removals++
				want = []string{"image", "rm", "skillhub-search-proof/" + image + ":owned-probe"}
			}
		}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("Docker command = %q, want %q", args, want)
		}
	}
	if (builds == 1) != wantBuild || builds > 1 || (!wantBuild && removals != 0) {
		t.Fatalf("build calls = %d, removal calls = %d", builds, removals)
	}
}
