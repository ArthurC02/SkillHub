package catalog

import (
	"encoding/json"
	"slices"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func scriptPresence(scan []byte) *bool {
	var report map[string]json.RawMessage
	_ = json.Unmarshal(scan, &report)
	raw, recorded := report["codes"]
	if !recorded {
		return nil
	}
	var codes []string
	_ = json.Unmarshal(raw, &codes)
	present := slices.Contains(codes, skillpkg.CodeScriptFile) || slices.Contains(codes, skillpkg.CodeEmbeddedScript)
	return &present
}
