package sandbox

import (
	"path"
	"strings"
)

func (r RunRequest) DatasetNames() map[string]string {
	out := map[string]string{}
	for _, ref := range r.TestCase.DatasetRefs {
		if ref.ObjectKey == "" {
			continue
		}
		name := path.Base(strings.ReplaceAll(ref.FileName, "\\", "/"))
		if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "-") {
			continue
		}
		out[ref.ObjectKey] = name
	}
	return out
}
