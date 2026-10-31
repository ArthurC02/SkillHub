package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const fixProofThreshold = 0.35

var (
	fixSubject    = regexp.MustCompile(`^fix(\(|!?:)`)
	fixProofWords = regexp.MustCompile(`(?i)mutant|mutation|突變|變紅|turns? (it )?red|went red|goes red|fail(s|ed)? without|revert(ing|ed)? the|red before|reverted`)
)

var fixProofQuestion = decisionQuestion{
	Type: "noul",
	Instructions: "Does the commit message at `message` report that a test, check, build or gate was observed failing without the change: " +
		"before the fix, on an earlier commit, or with the fix undone or broken on purpose?",
	Criteria: map[string][]string{
		"yes": {
			"says a new or existing test failed before the change, or fails when the fix is reverted, removed or mutated",
			"says CI, a lint, audit, build or contract check failed on an earlier commit and this change makes it pass",
			"describes reproducing the original failure",
		},
		"no": {
			"only lists what now passes (test counts, browsers, lint clean) without any observed failure",
			"only describes the defect and the new behaviour",
			"is a subject line with no body",
		},
	},
}

func fixProof(root string, args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: devctl fix-proof message-file")
	}
	file := args[0]
	if !filepath.IsAbs(file) {
		file = filepath.Join(root, file)
	}
	message, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if warning := fixProofWarning(string(message), newDecisionJudge(root)); warning != "" {
		fmt.Fprintln(out, statusWarn, warning)
	}
	return nil
}

func fixProofWarning(raw string, judge decisionJudge) string {
	message := commitMessageText(raw)
	if !fixSubject.MatchString(message) || fixProofWords.MatchString(message) || !judge.hasKey() {
		return ""
	}
	state := struct {
		Message string `json:"message"`
	}{message}
	answers, err := judge.ask(state, map[string]decisionQuestion{"proof": fixProofQuestion})
	if err != nil {
		return fmt.Sprintf("fix-proof advice skipped (%v)", err)
	}
	if answers["proof"] >= fixProofThreshold {
		return ""
	}
	return fmt.Sprintf("this fix commit reports no test seen failing without the change (%.2f): revert the fix line, "+
		"watch its test turn red, restore it, and record that red in the next commit message", answers["proof"])
}

func commitMessageText(raw string) string {
	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "# ------------------------ >8") {
			break
		}
		if !strings.HasPrefix(line, "#") {
			kept = append(kept, line)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}
