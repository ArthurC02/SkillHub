package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const generationLockMaxAge = 2 * time.Hour

type generationLock struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

type generatedTree struct {
	root  string
	files map[string]string
}

type generationOutput struct {
	label  string
	source string
	target string
}

func generate(root string, args []string, out io.Writer) error {
	check := false
	scope := "all"
	for _, arg := range args {
		switch arg {
		case "--check":
			check = true
		case "--scope=sql":
			scope = "sql"
		case "--scope=openapi":
			scope = "openapi"
		case "--scope=all":
			scope = "all"
		default:
			return fmt.Errorf("unknown gen option %q", arg)
		}
	}

	release, err := acquireGenerationLock(root, time.Now())
	if err != nil {
		return err
	}
	defer release()

	toolchain, err := parseToolchain(filepath.Join(root, "tools", "toolchain.yaml"))
	if err != nil {
		return err
	}
	scratch, err := os.MkdirTemp(filepath.Join(root, ".devctl"), "generate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)

	var outputs []generationOutput
	images, err := parseManifestSection(filepath.Join(root, "tools", "toolchain.yaml"), "images")
	if err != nil {
		return err
	}
	if scope == "all" || scope == "sql" {
		sqlOut, err := generateSQL(root, scratch, images["sqlc"], out)
		if err != nil {
			return err
		}
		outputs = append(outputs, generationOutput{
			label:  "sqlc",
			source: sqlOut,
			target: filepath.Join(root, "services", "platform", "internal", "platform", "db", "gen"),
		})
	}
	if scope == "all" || scope == "openapi" {
		openAPIOutputs, err := generateOpenAPI(root, scratch, toolchain, images, out)
		if err != nil {
			return err
		}
		outputs = append(outputs, openAPIOutputs...)
	}

	type pendingOutput struct {
		generationOutput
		drift []string
	}
	var pending []pendingOutput
	for _, output := range outputs {
		drift, err := compareTrees(output.source, output.target)
		if err != nil {
			return err
		}
		if len(drift) == 0 {
			fmt.Fprintf(out, "%s: generated output is current\n", output.label)
			continue
		}
		pending = append(pending, pendingOutput{generationOutput: output, drift: drift})
	}
	if check && len(pending) > 0 {
		for _, output := range pending {
			for _, path := range output.drift {
				fmt.Fprintln(out, "DRIFT", output.label, filepath.ToSlash(path))
			}
		}
		return errors.New("generated output is stale; run task gen and commit the result")
	}
	for _, output := range pending {
		if err := atomicReplaceDir(output.source, output.target); err != nil {
			return fmt.Errorf("replace %s output: %w", output.label, err)
		}
		for _, path := range output.drift {
			fmt.Fprintln(out, "UPDATED", output.label, filepath.ToSlash(path))
		}
	}
	return nil
}

func acquireGenerationLock(root string, now time.Time) (func(), error) {
	dir := filepath.Join(root, ".devctl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "generate.lock")
	create := func() (*os.File, error) {
		return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	}
	file, err := create()
	if errors.Is(err, os.ErrExist) {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("generation lock exists but cannot be read: %w", readErr)
		}
		var existing generationLock
		if json.Unmarshal(data, &existing) == nil && now.Sub(existing.CreatedAt) > generationLockMaxAge {
			if removeErr := os.Remove(path); removeErr != nil {
				return nil, fmt.Errorf("remove stale generation lock: %w", removeErr)
			}
			file, err = create()
		} else {
			return nil, fmt.Errorf("generation is already running (%s); shared worktree allows one writer", strings.TrimSpace(string(data)))
		}
	}
	if err != nil {
		return nil, err
	}
	lock := generationLock{PID: os.Getpid(), CreatedAt: now.UTC()}
	if err := json.NewEncoder(file).Encode(lock); err != nil {
		file.Close()
		os.Remove(path)
		return nil, err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

func generateSQL(root, scratch, image string, out io.Writer) (string, error) {
	if image == "" {
		return "", errors.New("sqlc image is missing from tools/toolchain.yaml")
	}
	config := `version: "2"
sql:
  - engine: postgresql
    schema: ../../db/migrations
    queries: ../../db/queries
    gen:
      go:
        package: gen
        out: sqlc-out
        sql_package: pgx/v5
        emit_pointers_for_null_types: true
`
	if err := os.WriteFile(filepath.Join(scratch, "sqlc.yaml"), []byte(config), 0o600); err != nil {
		return "", err
	}
	relScratch, err := filepath.Rel(root, scratch)
	if err != nil {
		return "", err
	}
	mount := root + ":/src"
	args := []string{"run", "--rm"}
	args = append(args, dockerUserArgs()...)
	args = append(args,
		"-v", mount,
		"-w", "/src/"+filepath.ToSlash(relScratch),
		image,
		"generate",
	)
	cmd := exec.Command("docker", args...)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("sqlc generation: %w", err)
	}
	generated := filepath.Join(scratch, "sqlc-out")
	if _, err := os.Stat(generated); err != nil {
		return "", fmt.Errorf("sqlc produced no output: %w", err)
	}
	return generated, nil
}

func compareTrees(generated, committed string) ([]string, error) {
	left, err := hashTree(generated)
	if err != nil {
		return nil, err
	}
	right, err := hashTree(committed)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	paths := map[string]struct{}{}
	for path := range left.files {
		paths[path] = struct{}{}
	}
	for path := range right.files {
		paths[path] = struct{}{}
	}
	var drift []string
	for path := range paths {
		if left.files[path] != right.files[path] {
			drift = append(drift, path)
		}
	}
	sort.Strings(drift)
	return drift, nil
}

func hashTree(root string) (generatedTree, error) {
	tree := generatedTree{root: root, files: map[string]string{}}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".pyc") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		tree.files[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	return tree, err
}

func atomicReplaceDir(source, target string) error {
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	backup := target + fmt.Sprintf(".devctl-backup-%d", time.Now().UnixNano())
	hadTarget := false
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, backup); err != nil {
			return err
		}
		hadTarget = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		if hadTarget {
			if restoreErr := os.Rename(backup, target); restoreErr != nil {
				return fmt.Errorf("install generated tree: %v; rollback also failed: %v; original remains at %s", err, restoreErr, backup)
			}
		}
		return err
	}
	if hadTarget {
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
	}
	return nil
}
