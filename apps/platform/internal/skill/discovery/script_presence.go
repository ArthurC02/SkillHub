package catalog

import (
	"encoding/json"
	"slices"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func scriptPresence(scan []byte) *bool {
	var report map[string]json.RawMessage
	if err := json.Unmarshal(scan, &report); err != nil {
		return nil
	}
	raw, recorded := report["codes"]
	if !recorded {
		return nil
	}
	var codes []string
	if err := json.Unmarshal(raw, &codes); err != nil {
		return nil
	}
	present := slices.Contains(codes, skillpkg.CodeScriptFile) || slices.Contains(codes, skillpkg.CodeEmbeddedScript)
	return &present
}
