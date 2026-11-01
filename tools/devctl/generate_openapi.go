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

func openAPIGenerationJobs(root, scratch string, toolchain, images map[string]string) ([]generationJob, error) {
	tsImage := images["ts_codegen"]
	pyImage := images["python_codegen"]
	goImage := images["go_codegen"]
	if tsImage == "" || pyImage == "" || goImage == "" {
		return nil, errors.New("OpenAPI codegen images are missing from tools/toolchain.yaml")
	}
	if toolchain["datamodel_code_generator"] == "" {
		return nil, errors.New("datamodel-code-generator version is missing from tools/toolchain.yaml")
	}

	tsDone := make(chan struct{})
	var ts generationOutput
	var tsErr error
	single := func(generate func(io.Writer) (generationOutput, error)) generationJob {
		return func(out io.Writer) ([]generationOutput, error) {
			output, err := generate(out)
			if err != nil {
				return nil, err
			}
			return []generationOutput{output}, nil
		}
	}
	return []generationJob{
		single(func(out io.Writer) (generationOutput, error) {
			defer close(tsDone)
			ts, tsErr = generateTypeScriptOpenAPI(root, scratch, tsImage, out)
			return ts, tsErr
		}),
		single(func(out io.Writer) (generationOutput, error) {
			return generatePythonOpenAPI(root, scratch, pyImage, out)
		}),
		single(func(out io.Writer) (generationOutput, error) {
			return generateGoOpenAPI(root, scratch, goImage, out)
		}),
		single(func(out io.Writer) (generationOutput, error) {
			output, err := generateContractLimits(root, scratch, out)
			<-tsDone
			if tsErr != nil {
				return generationOutput{}, errors.New("contract limits generation: skipped because TypeScript OpenAPI generation failed")
			}
			if err != nil {
				return generationOutput{}, err
			}
			return output, os.Rename(filepath.Join(output.source, limitsTSFile), filepath.Join(ts.source, limitsTSFile))
		}),
	}, nil
}

const limitsTSFile = "limits.ts"

func generateContractLimits(root, scratch string, out io.Writer) (generationOutput, error) {
	goRoot := filepath.Join(scratch, "contractlimits")
	if err := os.MkdirAll(goRoot, 0o755); err != nil {
		return generationOutput{}, err
	}
	cmd := exec.Command("go", "run", "./cmd/contractlimits",
		"-contract", filepath.Join(root, "contracts", "openapi", "public.yaml"),
		"-go", filepath.Join(goRoot, "limits_gen.go"),
		"-ts", filepath.Join(goRoot, limitsTSFile))
	cmd.Dir = filepath.Join(root, "apps", "platform")
	if output, err := cmd.CombinedOutput(); err != nil {
		_, _ = out.Write(output)
		return generationOutput{}, fmt.Errorf("contract limits generation: %w", err)
	}
	fmt.Fprintln(out, "contract limits generation: complete")
	return generationOutput{
		label:  "contract-limits",
		source: goRoot,
		target: filepath.Join(root, "apps", "platform", "internal", "shared", "contractlimits", dirGen),
	}, nil
}

func generateTypeScriptOpenAPI(root, scratch, tsImage string, out io.Writer) (generationOutput, error) {
	tsRoot := filepath.Join(scratch, "typescript")
	if err := os.MkdirAll(tsRoot, 0o755); err != nil {
		return generationOutput{}, err
	}
	if err := buildCodegenImage("TypeScript codegen image build", filepath.Join(root, "tools", "codegen", "ts"), tsImage, out); err != nil {
		return generationOutput{}, err
	}
	tsArgs := []string{cmdRun, dockerFlagRm}
	tsArgs = append(tsArgs, dockerUserArgs()...)
	tsArgs = append(tsArgs,
		"-v", root+":/src",
		"-w", "/src",
		tsImage,
		"/src/contracts/openapi/public.yaml",
		containerPath(root, tsRoot),
	)
	if err := runDocker("TypeScript OpenAPI generation", tsArgs, out); err != nil {
		return generationOutput{}, err
	}
	if err := validateGeneratedContent(tsRoot, root); err != nil {
		return generationOutput{}, err
	}
	return generationOutput{
		label:  "typescript-openapi",
		source: tsRoot,
		target: filepath.Join(root, "packages", "api-client-ts", "src", "generated"),
	}, nil
}

func generatePythonOpenAPI(root, scratch, pyImage string, out io.Writer) (generationOutput, error) {
	if err := buildCodegenImage("Python codegen image build", filepath.Join(root, "tools", "codegen", "python"), pyImage, out); err != nil {
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
	if err := buildCodegenImage("Go codegen image build", filepath.Join(root, "tools", "codegen", "go"), goImage, out); err != nil {
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

const envCodegenImagesPrebuilt = "SKILLHUB_CODEGEN_IMAGES_PREBUILT"

func buildCodegenImage(label, contextDir, image string, out io.Writer) error {
	if reuseCodegenImage(os.Getenv(envCodegenImagesPrebuilt), image, dockerImagePresent) {
		fmt.Fprintln(out, label+": reused "+image)
		return nil
	}
	return runDocker(label, []string{
		cmdBuild, flagQuiet,
		"-f", filepath.Join(contextDir, "Dockerfile"),
		"-t", image,
		contextDir,
	}, out)
}

func reuseCodegenImage(prebuilt, image string, present func(string) bool) bool {
	return prebuilt == "true" && present(image)
}

func dockerImagePresent(image string) bool {
	return exec.Command("docker", "image", "inspect", image).Run() == nil
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
