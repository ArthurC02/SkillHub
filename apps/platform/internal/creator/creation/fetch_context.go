package creation

import (
	"encoding/json"
	"slices"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation/webfetch"
)

const toolMessageRole = "tool"

func fetchedMessages(messages []Message, fetched *fetchedPage, mask func(string) string) []Message {
	if fetched == nil || fetched.record.Status != "ok" || len(messages) == 0 {
		return messages
	}
	last := len(messages) - 1
	if messages[last].Role != toolMessageRole || messages[last].Content != webfetch.Observation(fetched.record, "") {
		return messages
	}
	messages = slices.Clone(messages)
	messages[last].Content = webfetch.Observation(fetched.record, mask(fetched.text))
	return messages
}

func withoutFetchedText(messages []Message) []Message {
	clean := slices.Clone(messages)
	for i, message := range clean {
		if message.Role != toolMessageRole {
			continue
		}
		var observation map[string]json.RawMessage
		if json.Unmarshal([]byte(message.Content), &observation) != nil {
			continue
		}
		var fetch map[string]json.RawMessage
		if json.Unmarshal(observation["fetch"], &fetch) != nil || fetch == nil {
			continue
		}
		if _, hasText := fetch["text"]; hasText {
			fetch["text"] = json.RawMessage(`""`)
			observation["fetch"], _ = json.Marshal(fetch)
			encoded, _ := json.Marshal(observation)
			clean[i].Content = string(encoded)
		}
	}
	return clean
}
