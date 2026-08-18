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

func generateOpenAPI(root, scratch string, toolchain, images map[string]string, out io.Writer) ([]generationOutput, error) {
	tsImage := images["openapi_generator"]
	pyImage := images["python_codegen"]
	goImage := images["go_codegen"]
	if tsImage == "" || pyImage == "" || goImage == "" {
		return nil, errors.New("OpenAPI generator images are missing from tools/toolchain.yaml")
	}
	if toolchain["datamodel_code_generator"] == "" {
		return nil, errors.New("datamodel-code-generator version is missing from tools/toolchain.yaml")
	}

	tsRoot := filepath.Join(scratch, "typescript")
	if err := os.MkdirAll(tsRoot, 0o755); err != nil {
		return nil, err
	}
	tsArgs := []string{"run", "--rm"}
	tsArgs = append(tsArgs, dockerUserArgs()...)
	tsArgs = append(tsArgs,
		"-v", root+":/src",
		"-w", "/src",
		tsImage,
		"generate",
		"-i", "/src/contracts/openapi/public.yaml",
		"-g", "typescript-fetch",
		"-o", containerPath(root, tsRoot),
		"--global-property", "apis,models,supportingFiles,apiDocs=false,modelDocs=false,apiTests=false,modelTests=false",
		"--additional-properties", "supportsES6=true,useSingleRequestParameter=true,withInterfaces=true,npmName=@skillhub/api-client-ts",
	)
	if err := runCaptured("TypeScript OpenAPI generation", "docker", tsArgs, out); err != nil {
		return nil, err
	}
	tsSource := filepath.Join(tsRoot, "src")
	if err := validateGeneratedContent(tsSource, root); err != nil {
		return nil, err
	}

	buildArgs := []string{
		"build", "--quiet",
		"-f", filepath.Join(root, "tools", "codegen", "python", "Dockerfile"),
		"-t", pyImage,
		root,
	}
	if err := runCaptured("Python codegen image build", "docker", buildArgs, out); err != nil {
		return nil, err
	}
	pyRoot := filepath.Join(scratch, "python")
	if err := os.MkdirAll(pyRoot, 0o755); err != nil {
		return nil, err
	}
	pyArgs := []string{"run", "--rm"}
	pyArgs = append(pyArgs, dockerUserArgs()...)
	pyArgs = append(pyArgs,
		"-v", root+":/src",
		"-w", "/src",
		pyImage,
		"--input", "/src/contracts/openapi/llm-internal.yaml",
		"--input-file-type", "openapi",
		"--output", containerPath(root, filepath.Join(pyRoot, "models.py")),
		"--output-model-type", "pydantic_v2.BaseModel",
		"--encoding", "utf-8",
		"--disable-timestamp",
	)
	if err := runCaptured("Python OpenAPI generation", "docker", pyArgs, out); err != nil {
		return nil, err
	}
	init := "# Code generated boundary. models.py is replaced by `task gen:openapi`.\nfrom .models import *  # noqa: F403\n"
	if err := os.WriteFile(filepath.Join(pyRoot, "__init__.py"), []byte(init), 0o644); err != nil {
		return nil, err
	}
	if err := validateGeneratedContent(pyRoot, root); err != nil {
		return nil, err
	}

	goBuildArgs := []string{
		"build", "--quiet",
		"-f", filepath.Join(root, "tools", "codegen", "go", "Dockerfile"),
		"-t", goImage,
		root,
	}
	if err := runCaptured("Go codegen image build", "docker", goBuildArgs, out); err != nil {
		return nil, err
	}
	goRoot := filepath.Join(scratch, "go")
	if err := os.MkdirAll(goRoot, 0o755); err != nil {
		return nil, err
	}
	goArgs := []string{"run", "--rm"}
	goArgs = append(goArgs, dockerUserArgs()...)
	goArgs = append(goArgs,
		"-v", root+":/src",
		"-w", containerPath(root, goRoot),
		goImage,
		"--target", ".",
		"--package", "publicapi",
		"--config", "/src/tools/codegen/go/ogen.yaml",
		"/src/contracts/openapi/public.yaml",
	)
	if err := runCaptured("Go OpenAPI generation", "docker", goArgs, out); err != nil {
		return nil, err
	}
	if err := validateGeneratedContent(goRoot, root); err != nil {
		return nil, err
	}

	return []generationOutput{
		{
			label:  "typescript-openapi",
			source: tsSource,
			target: filepath.Join(root, "packages", "api-client-ts", "src", "generated"),
		},
		{
			label:  "python-openapi",
			source: pyRoot,
			target: filepath.Join(root, "packages", "api-stub-py", "src", "skillhub_api_stub", "generated"),
		},
		{
			label:  "go-openapi",
			source: goRoot,
			target: filepath.Join(root, "services", "platform", "internal", "api", "gen"),
		},
	}, nil
}

func runCaptured(label, name string, args []string, out io.Writer) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			_, _ = out.Write(output)
		}
		return fmt.Errorf("%s: %w", label, err)
	}
	fmt.Fprintln(out, label+": complete")
	return nil
}

func containerPath(root, hostPath string) string {
	rel, err := filepath.Rel(root, hostPath)
	if err != nil {
		panic(err)
	}
	return "/src/" + filepath.ToSlash(rel)
}

func validateGeneratedContent(root, repoRoot string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if strings.Contains(text, repoRoot) || strings.Contains(text, filepath.ToSlash(repoRoot)) {
			return fmt.Errorf("generated file contains repository absolute path: %s", path)
		}
		head := text
		if lines := strings.Split(text, "\n"); len(lines) > 20 {
			head = strings.Join(lines[:20], "\n")
		}
		lower := strings.ToLower(head)
		if strings.Contains(lower, "generated at") || strings.Contains(lower, "timestamp:") {
			return fmt.Errorf("generated file contains nondeterministic timestamp header: %s", path)
		}
		return nil
	})
}
