package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	decisionJudgeKeyName   = "JEV_API_KEY"
	decisionJudgeModel     = "jev-latest"
	decisionLogThreshold   = 0.45
	decisionJudgeBatchSize = 10
	decisionJudgeTimeout   = 10 * time.Second
)

var decisionJudgeBaseURL = "https://api.typesafe.ai"

type decisionJudge struct {
	baseURL string
	key     string
	client  *http.Client
}

type commentBlock struct {
	file string
	line int
	text string
}

func newDecisionJudge(root string) decisionJudge {
	key := os.Getenv(decisionJudgeKeyName)
	if key == "" {
		if values, err := readDotEnv(filepath.Join(root, ".env")); err == nil {
			key = strings.Trim(values[decisionJudgeKeyName], `"'`)
		}
	}
	return decisionJudge{baseURL: decisionJudgeBaseURL, key: key, client: &http.Client{Timeout: decisionJudgeTimeout}}
}

func (j decisionJudge) hasKey() bool { return j.key != "" }

func isTestSource(file string) bool {
	base := path.Base(file)
	return strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		strings.HasPrefix(base, "test_") || base == "conftest.py" || strings.Contains(file, "/tests/") ||
		strings.Contains(file, "/providertest/")
}

func commentBlocks(file, src string, prefixes []string) []commentBlock {
	var blocks []commentBlock
	var lines []string
	start := 0
	flush := func() {
		if text := strings.TrimSpace(strings.Join(lines, "\n")); text != "" {
			blocks = append(blocks, commentBlock{file: file, line: start, text: text})
		}
		lines, start = nil, 0
	}
	for index, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if !hasAnyPrefix(trimmed, prefixes) {
			flush()
			continue
		}
		if commentMachineMarker.MatchString(trimmed) {
			continue
		}
		body := strings.TrimSpace(strings.TrimLeft(trimmed, "/#*-"))
		if body == "" {
			continue
		}
		if start == 0 {
			start = index + 1
		}
		lines = append(lines, body)
	}
	flush()
	return blocks
}

func newCommentBlocks(root, rangeSpec string) ([]commentBlock, error) {
	names, err := gitOutput(root, "diff", "--name-only", "--diff-filter=ACMR", rangeSpec)
	if err != nil {
		return nil, err
	}
	end, start := rangeEnd(rangeSpec), rangeStart(rangeSpec)
	var blocks []commentBlock
	for _, file := range lines(names) {
		prefixes, ok := commentPrefixesFor(path.Base(file))
		if !ok || !commentBudgetCovers(file) || isTestSource(file) {
			continue
		}
		blob, err := gitOutput(root, "cat-file", "blob", end+":"+file)
		if err != nil {
			return nil, err
		}
		before := map[string]bool{}
		if base, err := gitOutput(root, "cat-file", "blob", start+":"+file); err == nil {
			for _, block := range commentBlocks(file, base, prefixes) {
				before[block.text] = true
			}
		}
		for _, block := range commentBlocks(file, blob, prefixes) {
			if !before[block.text] {
				blocks = append(blocks, block)
			}
		}
	}
	return blocks, nil
}

func rangeStart(rangeSpec string) string {
	if i := strings.Index(rangeSpec, ".."); i > 0 {
		return rangeSpec[:i]
	}
	return rangeSpec + "^"
}

func (j decisionJudge) decisionLogProbabilities(blocks []commentBlock) ([]float64, error) {
	probabilities := make([]float64, 0, len(blocks))
	for i := 0; i < len(blocks); i += decisionJudgeBatchSize {
		batch := blocks[i:min(i+decisionJudgeBatchSize, len(blocks))]
		answers, err := j.judgeBatch(batch)
		if err != nil {
			return nil, err
		}
		probabilities = append(probabilities, answers...)
	}
	return probabilities, nil
}

type decisionQuestion struct {
	Type         string              `json:"type"`
	Instructions string              `json:"instructions"`
	Criteria     map[string][]string `json:"criteria"`
}

var decisionLogCriteria = map[string][]string{
	"yes": {
		"tells how the code came to be this way: what it used to do, when it changed, a bug or incident that was found, what was missing \"until now\"",
		"records who or what decided it: an owner, a ruling, a review, a design document, an architecture decision, a requirement, or a memo",
		"argues for a policy or product choice, or explains why one design was preferred over another",
	},
	"no": {
		"describes what the code does, its inputs, outputs or contract",
		"explains how an algorithm or mechanism works",
		"states an invariant, precondition or the technical consequence of the current code, such as what would break, leak, deadlock or overflow without it",
	},
}

func (j decisionJudge) judgeBatch(batch []commentBlock) ([]float64, error) {
	type comment struct {
		File string `json:"file"`
		Text string `json:"text"`
	}
	state := struct {
		Comments []comment `json:"comments"`
	}{}
	questions := map[string]decisionQuestion{}
	for i, block := range batch {
		state.Comments = append(state.Comments, comment{File: block.file, Text: block.text})
		questions[fmt.Sprintf("c%d", i)] = decisionQuestion{
			Type: "noul",
			Instructions: fmt.Sprintf("Is the source-code comment at `comments[%d].text` a work log or decision record "+
				"rather than an explanation of the code as it is now?", i),
			Criteria: decisionLogCriteria,
		}
	}
	answers, err := j.ask(state, questions)
	if err != nil {
		return nil, err
	}
	probabilities := make([]float64, len(batch))
	for i := range batch {
		probabilities[i] = answers[fmt.Sprintf("c%d", i)]
	}
	return probabilities, nil
}

func (j decisionJudge) ask(state any, questions map[string]decisionQuestion) (map[string]float64, error) {
	body, err := json.Marshal(map[string]any{"model": decisionJudgeModel, "state": state, "questions": questions})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, j.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+j.key)
	request.Header.Set("Content-Type", "application/json")
	response, err := j.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("judge answered %s", response.Status)
	}
	var decoded struct {
		Answers map[string]struct {
			Noul *float64 `json:"noul"`
		} `json:"answers"`
	}
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	probabilities := make(map[string]float64, len(questions))
	for id := range questions {
		answer, ok := decoded.Answers[id]
		if !ok || answer.Noul == nil {
			return nil, fmt.Errorf("judge left question %s unanswered", id)
		}
		probabilities[id] = *answer.Noul
	}
	return probabilities, nil
}

func committedDecisionCommentWarnings(root, rangeSpec string, judge decisionJudge) ([]string, error) {
	if !judge.hasKey() {
		return nil, nil
	}
	blocks, err := newCommentBlocks(root, rangeSpec)
	if err != nil || len(blocks) == 0 {
		return nil, err
	}
	probabilities, err := judge.decisionLogProbabilities(blocks)
	if err != nil {
		return []string{fmt.Sprintf("decision-comment advice skipped (%v); comment-budget still applies", err)}, nil
	}
	var warnings []string
	for i, block := range blocks {
		if probabilities[i] >= decisionLogThreshold {
			warnings = append(warnings, fmt.Sprintf(
				"%s:%d reads like a decision record (%.2f): keep how the code works in the comment and move why it was decided to the commit message",
				block.file, block.line, probabilities[i]))
		}
	}
	return warnings, nil
}
