package publishing

import (
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

type NameProblem string

const (
	NameShape    NameProblem = "shape"
	NameReserved NameProblem = "reserved"
)

var reservedPublisherNames = []string{
	"skillhub", "admin", "administrator", "official", "support", "security", "root", "system",
	"platform", "staff", "operator", "moderator", "help", "api", "www",
	"anthropic", "claude", "openai", "chatgpt", "codex", "google", "gemini", "microsoft", "copilot",
	"github", "meta", "llama", "mistral", "deepseek", "qwen", "xai", "grok",
}

func publisherNameProblem(name string) NameProblem {
	if !skillpkg.ValidName(name) {
		return NameShape
	}
	withoutHyphens := strings.ReplaceAll(name, "-", "")
	for _, reserved := range reservedPublisherNames {
		if withoutHyphens == reserved {
			return NameReserved
		}
	}
	return ""
}

func publicationNameProblem(name string) NameProblem {
	if !skillpkg.ValidName(name) {
		return NameShape
	}
	return ""
}
