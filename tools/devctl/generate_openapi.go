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

	ts, err := generateTypeScriptOpenAPI(root, scratch, tsImage, out)
	if err != nil {
		return nil, err
	}
	py, err := generatePythonOpenAPI(root, scratch, pyImage, out)
	if err != nil {
		return nil, err
	}
	goOutput, err := generateGoOpenAPI(root, scratch, goImage, out)
	if err != nil {
		return nil, err
	}
	return []generationOutput{ts, py, goOutput}, nil
}

func generateTypeScriptOpenAPI(root, scratch, tsImage string, out io.Writer) (generationOutput, error) {
	tsRoot := filepath.Join(scratch, "typescript")
	if err := os.MkdirAll(tsRoot, 0o755); err != nil {
		return generationOutput{}, err
	}
	tsArgs := []string{cmdRun, dockerFlagRm}
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
	if err := runDocker("TypeScript OpenAPI generation", tsArgs, out); err != nil {
		return generationOutput{}, err
	}
	tsSource := filepath.Join(tsRoot, "src")
	if err := validateGeneratedContent(tsSource, root); err != nil {
		return generationOutput{}, err
	}
	return generationOutput{
		label:  "typescript-openapi",
		source: tsSource,
		target: filepath.Join(root, "packages", "api-client-ts", "src", "generated"),
	}, nil
}

func generatePythonOpenAPI(root, scratch, pyImage string, out io.Writer) (generationOutput, error) {
	buildArgs := []string{
		cmdBuild, flagQuiet,
		"-f", filepath.Join(root, "tools", "codegen", "python", "Dockerfile"),
		"-t", pyImage,
		filepath.Join(root, "tools", "codegen", "python"),
	}
	if err := runDocker("Python codegen image build", buildArgs, out); err != nil {
		return generationOutput{}, err
	}
	pyRoot := filepath.Join(scratch, "python")
	if err := os.MkdirAll(pyRoot, 0o755); err != nil {
		return generationOutput{}, err
	}
	pyArgs := []string{cmdRun, dockerFlagRm}
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
	if err := runDocker("Python OpenAPI generation", pyArgs, out); err != nil {
		return generationOutput{}, err
	}
	init := "# Code generated boundary. models.py is replaced by `task gen:openapi`.\nfrom .models import *  # noqa: F403\n"
	if err := os.WriteFile(filepath.Join(pyRoot, "__init__.py"), []byte(init), 0o644); err != nil {
		return generationOutput{}, err
	}
	if err := validateGeneratedContent(pyRoot, root); err != nil {
		return generationOutput{}, err
	}
	return generationOutput{
		label:  "python-openapi",
		source: pyRoot,
		target: filepath.Join(root, "packages", "api-stub-py", "src", "skillhub_api_stub", "generated"),
	}, nil
}

func generateGoOpenAPI(root, scratch, goImage string, out io.Writer) (generationOutput, error) {
	goBuildArgs := []string{
		cmdBuild, flagQuiet,
		"-f", filepath.Join(root, "tools", "codegen", "go", "Dockerfile"),
		"-t", goImage,
		filepath.Join(root, "tools", "codegen", "go"),
	}
	if err := runDocker("Go codegen image build", goBuildArgs, out); err != nil {
		return generationOutput{}, err
	}
	goRoot := filepath.Join(scratch, "go")
	if err := os.MkdirAll(goRoot, 0o755); err != nil {
		return generationOutput{}, err
	}
	goArgs := []string{cmdRun, dockerFlagRm}
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
	if err := runDocker("Go OpenAPI generation", goArgs, out); err != nil {
		return generationOutput{}, err
	}
	if err := validateGeneratedContent(goRoot, root); err != nil {
		return generationOutput{}, err
	}
	return generationOutput{
		label:  "go-openapi",
		source: goRoot,
		target: filepath.Join(root, "apps", "platform", "internal", "entrypoint", "api", dirGen),
	}, nil
}

func runDocker(label string, args []string, out io.Writer) error {
	cmd := exec.Command("docker", args...)
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

const generatedHeaderScanLines = 20

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
		if lines := strings.Split(text, "\n"); len(lines) > generatedHeaderScanLines {
			head = strings.Join(lines[:generatedHeaderScanLines], "\n")
		}
		lower := strings.ToLower(head)
		if strings.Contains(lower, "generated at") || strings.Contains(lower, "timestamp:") {
			return fmt.Errorf("generated file contains nondeterministic timestamp header: %s", path)
		}
		return nil
	})
}
