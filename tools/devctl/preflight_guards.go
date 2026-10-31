package main

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	guardsPluginDir       = ".claude/skills/skillhub-guards"
	guardsReportTailLines = 20
)

var guardsTestCommand = []string{"claude", "plugin", "test", guardsPluginDir}

func committedGuardsTestProblems(root, rangeSpec string, command []string) (problems, warnings []string, err error) {
	names, err := gitOutput(root, "diff", "--name-only", rangeSpec, "--", guardsPluginDir)
	if err != nil || len(lines(names)) == 0 {
		return nil, nil, err
	}
	if !commandInstalled(command[0]) {
		return nil, []string{fmt.Sprintf("%s changed but `%s` is not installed, so its tests did not run and CI does not run them either",
			guardsPluginDir, strings.Join(command, " "))}, nil
	}
	uncommitted, err := gitOutput(root, "diff", "--name-only", rangeEnd(rangeSpec), "--", guardsPluginDir)
	if err != nil {
		return nil, nil, err
	}
	if len(lines(uncommitted)) > 0 {
		warnings = append(warnings, fmt.Sprintf("%s in the working tree differs from the pushed commit; its tests ran on the working tree",
			guardsPluginDir))
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = root
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		problems = append(problems, fmt.Sprintf("%s changed and `%s` failed (%v):\n%s",
			guardsPluginDir, strings.Join(command, " "), runErr, lastLines(string(output), guardsReportTailLines)))
	}
	return problems, warnings, nil
}

func lastLines(text string, n int) string {
	all := strings.Split(strings.TrimRight(text, "\n"), "\n")
	return strings.Join(all[max(0, len(all)-n):], "\n")
}

func commandInstalled(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
