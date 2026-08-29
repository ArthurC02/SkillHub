package main

// The golden set measures the production retrieval path only while its Python
// copy of `embeddingText` still says what the Go one says.
//
// tools/goldenset/evaluate.py builds the indexed string in
// `enriched_index_text`, and its own docstring states the arrangement: 「
// Transcribed from apps/platform/internal/skill/admission/enrich.go:
// embeddingText … If this drifts from the Go side, the golden set stops
// measuring the production retrieval path — which is the whole point of
// --index-mode enriched, so keep the two in step.」
//
// `--selfcheck` has an assertion pinning the exact expected output, which proves
// the Python side did not change WITHOUT NOTICING. It compares nothing against
// Go. Editing `embeddingText` — reordering the tag buckets, dropping the English
// half of a task example, changing the separator — turns nothing red, and the
// recall@5 = 48/48 that gate-test/README.md §3.1 marks ✅ silently becomes a
// number about a program nobody runs.
//
// WHAT THIS IS AND WHY IT IS A DIGEST. There is no shared artefact to compare:
// one side is Go building a string from struct fields, the other is Python
// building it from a dict, and any structural comparison would be a parser for
// two languages guessing at equivalence. What both sides DO have is a body that
// somebody has to read to keep them in step. So both bodies are pinned by
// digest, comments and blank lines stripped, and changing either one turns this
// red with an instruction: read both, confirm they still produce the same
// string, then re-pin.
//
// That is a deliberately dumb check and it is honest about what it buys. It
// cannot tell a fixing edit from a breaking one. What it buys is that the edit
// cannot be silent, which is the whole failure — and re-pinning costs one line
// and requires the person to have looked.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	goldensetPython = "tools/goldenset/evaluate.py"
	goldensetGo     = "apps/platform/internal/skill/admission/enrich.go"
)

// The two sides, as the smallest span of each file that decides the string.
//
// The Go side is three functions, not one: `embeddingText` joins the parts, and
// `flatTags` and `joinTaskExamples` decide what two of those parts contain.
// `enriched_index_text` does all three jobs, so all three have to be watched or
// the bucket order could change under a green check.
var goldensetSpans = []struct {
	file, name, start, end string
	digest                 string
}{{
	file: goldensetGo, name: "embeddingText", start: "func embeddingText(", end: "\n}",
	digest: "",
}, {
	file: goldensetGo, name: "flatTags", start: "func (e enrichment) flatTags()", end: "\n}",
	digest: "",
}, {
	file: goldensetGo, name: "joinTaskExamples", start: "func joinTaskExamples(", end: "\n}",
	digest: "",
}, {
	file: goldensetPython, name: "enriched_index_text", start: "def enriched_index_text(", end: "\ndef ",
	digest: "",
}}

// The pinned digests. Filled by the test that first computed them; re-pin in the
// same commit that changes either side, after confirming the two still agree.
//
// Set by: A8, 2026-08-29, from the tree at 331bd90.
var goldensetPinned = map[string]string{
	"embeddingText":       "af940c00a9126201aed3908e9fd97a55bc65cca965fe69df71aea80be80461de",
	"flatTags":            "f4ecd7636059797e7eeb0067e57697ef742d7da27b4c471a0915ac641ef176cc",
	"joinTaskExamples":    "26eb2f141a252b10612598270c606bbaeb901e7715c11903b86406871e8edae0",
	"enriched_index_text": "3df8036c499a9a9565d357fae542ab14be4b4bd91b44dc9478c6d9798060af67",
}

var (
	goCommentLine = regexp.MustCompile(`^\s*//`)
	pyCommentLine = regexp.MustCompile(`^\s*#`)
)

func goldensetMirrorProblems(root string) []string {
	var problems []string
	for _, span := range goldensetSpans {
		body, err := extractSpan(filepath.Join(root, filepath.FromSlash(span.file)), span.start, span.end)
		if err != nil {
			problems = append(problems, fmt.Sprintf("goldenset-mirror: %s: %v", span.file, err))
			continue
		}
		got := goldensetDigest(span.file, body)
		want, pinned := goldensetPinned[span.name]
		if !pinned || want == "" {
			problems = append(problems, fmt.Sprintf(
				"goldenset-mirror: %s in %s has no pinned digest in tools/devctl/goldenset_mirror.go; "+
					"it is %s today", span.name, span.file, got))
			continue
		}
		if got != want {
			problems = append(problems, fmt.Sprintf(
				"goldenset-mirror: %s in %s changed (pinned %s, now %s). It is one half of a hand-copied "+
					"pair: %s builds the indexed string, %s transcribes it, and the golden set's "+
					"recall@5 = 48/48 (gate-test/README.md §3.1) only measures the production retrieval "+
					"path while the two agree. Read both, confirm they still produce the same string for "+
					"the same input, then re-pin this digest in the same commit",
				span.name, span.file, want[:12], got[:12], goldensetGo, goldensetPython))
		}
	}
	return problems
}

// extractSpan returns the text from the first occurrence of start up to the next
// occurrence of end after it. Zero matches is a loud failure: a span that has
// moved is a span nothing is watching.
func extractSpan(path, start, end string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	text := string(data)
	from := strings.Index(text, start)
	if from < 0 {
		return "", fmt.Errorf("no longer contains %q; the definition moved or was renamed, and this "+
			"comparison has lost that half of its subject", start)
	}
	rest := text[from+len(start):]
	to := strings.Index(rest, end)
	if to < 0 {
		return "", fmt.Errorf("%q is not followed by %q; the span cannot be bounded", start, end)
	}
	return text[from : from+len(start)+to], nil
}

// goldensetDigest hashes the code lines only. Comments are stripped because both
// bodies carry long explanations that are edited often and decide nothing, and a
// check that goes red for a typo fix is a check someone deletes.
func goldensetDigest(file, body string) string {
	comment := goCommentLine
	if strings.HasSuffix(file, ".py") {
		comment = pyCommentLine
	}
	var kept []string
	inDocstring := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		// Python docstrings are comments that are not `#` lines. The one here is
		// the transcription note itself — prose, edited often, decides nothing.
		if comment == pyCommentLine && strings.Count(line, `"""`) == 1 {
			inDocstring = !inDocstring
			continue
		}
		if inDocstring || trimmed == "" || comment.MatchString(line) {
			continue
		}
		kept = append(kept, trimmed)
	}
	sum := sha256.Sum256([]byte(strings.Join(kept, "\n")))
	return hex.EncodeToString(sum[:])
}
