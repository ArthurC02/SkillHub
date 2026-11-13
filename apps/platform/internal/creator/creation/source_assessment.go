package creation

import (
	"encoding/json"
	"path"
	"strings"
	"unicode/utf8"
)

const maxSourceRules = 64 // one-number: creationMaxSourceRules

const authorMessageRole = "user"

func sourceDraftHash(p Snapshot, skill GeneratedSkill) string {
	var authors []string
	for _, message := range p.Messages {
		if message.Role == authorMessageRole {
			authors = append(authors, message.Content)
		}
	}
	return digest(struct {
		Skill              GeneratedSkill
		Brief              string
		Criteria           []string
		Sample             string
		Diagram            *DiagramInterpretation
		DiagramDescription string
		DiagramFingerprint string
		Authors            []string
		Fetches            []Fetch
	}{skill, p.Brief, p.AcceptanceCriteria, p.SampleInput, p.DiagramInterpretation, p.DiagramDescription, p.DiagramFingerprint, authors, p.Fetches})
}

func sourcesChecked(e envelope) bool {
	return e.SourceCheckedHash != "" && e.Snapshot.Draft != nil && e.SourceCheckedHash == sourceDraftHash(e.Snapshot, e.Snapshot.Draft.Skill)
}

func screenSources(e envelope, r *StepResult) {
	if !submitsDraft(r) || r.Draft == nil {
		return
	}
	if r.SourceAssessment == nil && sourcesChecked(e) && digest(r.Draft) == digest(e.Snapshot.Draft.Skill) {
		return
	}
	sources := e.Snapshot
	if e.sourceMessages != nil {
		sources.Messages = e.sourceMessages
	}
	questions, valid := sourceQuestions(sources, r)
	if valid && len(questions) == 0 {
		return
	}
	r.Outcome, r.Draft, r.ToolIntent = outcomeClarification, nil, nil
	r.Brief, r.AcceptanceCriteria, r.SampleInput = "", nil, ""
	if !valid {
		r.Reason = reasonSourcesMissing
		return
	}
	const more = "其餘缺少的規則會在這批問題回答後繼續核對。"
	r.Message = ""
	for _, question := range questions {
		candidate := question
		if r.Message != "" {
			candidate = r.Message + "\n" + question
		}
		if utf8.RuneCountInString(candidate) > MaxTextRunes-utf8.RuneCountInString(more)-1 {
			r.Message += "\n" + more
			break
		}
		r.Message = candidate
	}
}

func sourceQuestions(p Snapshot, r *StepResult) ([]string, bool) {
	assessment := r.SourceAssessment
	if assessment == nil || assessment.Rules == nil || len(assessment.Rules) > maxSourceRules {
		return nil, false
	}
	var questions []string
	seen := map[string]bool{}
	for _, rule := range assessment.Rules {
		name := strings.TrimSpace(rule.Rule)
		if name == "" || utf8.RuneCountInString(rule.Rule) > 2000 || seen[name] {
			return nil, false
		}
		seen[name] = true
		if rule.Question != nil {
			question := strings.TrimSpace(*rule.Question)
			if question == "" || utf8.RuneCountInString(*rule.Question) > 2000 || rule.SourceMessageIndex != nil || rule.Quote != nil || rule.Path != nil {
				return nil, false
			}
			questions = append(questions, question)
			continue
		}
		if !sourceRuleHasEvidence(p, r.Draft, rule) {
			return nil, false
		}
	}
	return questions, true
}

func sourceRuleHasEvidence(p Snapshot, draft *GeneratedSkill, rule SourceRule) bool {
	if rule.Quote == nil || rule.Path == nil {
		return false
	}
	quote, name := *rule.Quote, *rule.Path
	if strings.TrimSpace(quote) == "" || utf8.RuneCountInString(quote) > 2000 || utf8.RuneCountInString(name) > 256 {
		return false
	}
	if !ruleFileIsPackaged(draft, name) {
		return false
	}
	if rule.SourceMessageIndex == nil {
		return strings.Contains(confirmedDiagramSource(p), quote)
	}
	index := *rule.SourceMessageIndex
	return index >= 0 && index < len(p.Messages) && strings.Contains(sourceText(p, p.Messages[index]), quote)
}

func ruleFileIsPackaged(draft *GeneratedSkill, name string) bool {
	if path.Clean(name) != name || strings.Contains(name, "\\") || (!strings.HasPrefix(name, "references/") && !strings.HasPrefix(name, "assets/")) || !strings.Contains(draft.Body, name) {
		return false
	}
	for _, file := range draft.Files {
		if file.Path == name && strings.TrimSpace(file.Content) != "" {
			return true
		}
	}
	return false
}

func confirmedDiagramSource(p Snapshot) string {
	if p.DiagramFingerprint == "" || !p.DiagramConfirmed || p.DiagramInterpretation == nil || !diagramSettled(p) {
		return ""
	}
	items := []string{p.DiagramDescription}
	items = append(items, p.DiagramInterpretation.Nodes...)
	items = append(items, p.DiagramInterpretation.Conditions...)
	items = append(items, p.DiagramInterpretation.Branches...)
	for _, uncertainty := range p.DiagramInterpretation.Uncertainties {
		items = append(items, uncertainty.Answer)
	}
	return strings.Join(items, "\n")
}

func sourceText(p Snapshot, message Message) string {
	if message.Role == authorMessageRole {
		return message.Content
	}
	if message.Role != toolMessageRole {
		return ""
	}
	var observation struct {
		Fetch struct {
			URL    string `json:"url"`
			Status string `json:"status"`
			SHA256 string `json:"sha256"`
			Bytes  int    `json:"bytes"`
			Text   string `json:"text"`
		} `json:"fetch"`
	}
	if json.Unmarshal([]byte(message.Content), &observation) != nil || observation.Fetch.Status != "ok" {
		return ""
	}
	fetch := observation.Fetch
	for _, record := range p.Fetches {
		if record.Status == "ok" && record.URL == fetch.URL && record.SHA256 != "" && record.SHA256 == fetch.SHA256 && record.Bytes > 0 && record.Bytes == fetch.Bytes {
			return fetch.Text
		}
	}
	return ""
}
