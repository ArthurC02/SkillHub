package creation

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func draftText(skill llmclient.GeneratedSkill) string {
	parts := []string{skill.Name, skill.Description, skill.Compatibility, skill.AllowedTools, skill.Body}
	for _, f := range skill.Files {
		parts = append(parts, f.Path, f.Content)
	}
	return strings.Join(parts, "\n")
}

func previousDraftText(d *Draft) string {
	if d == nil {
		return ""
	}
	return draftText(d.Skill)
}

func personText(messages []llmclient.CreationMessage) string {
	var b strings.Builder
	for _, m := range messages {
		if m.Role == "user" {
			b.WriteString(m.Content)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func missingDiagramNodes(understanding, body string) []string {
	var sections map[string][]string
	if json.Unmarshal([]byte(understanding), &sections) != nil {
		return nil
	}
	haystack := foldForMatch(body)
	var missing []string
	for _, node := range sections["nodes"] {
		if needle := foldForMatch(node); needle != "" && !strings.Contains(haystack, needle) {
			missing = append(missing, node)
		}
	}
	return missing
}

func foldForMatch(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
