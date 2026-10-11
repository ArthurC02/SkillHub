package eval

import (
	"net/http"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

type boundsView struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type checkKindView struct {
	Kind        string     `json:"kind"`
	Category    string     `json:"category"`
	Label       string     `json:"label"`
	Description string     `json:"description"`
	Values      boundsView `json:"values"`
	Children    boundsView `json:"children"`
	AtLeast     bool       `json:"at_least"`
}

func ListCheckKinds(w http.ResponseWriter, _ *http.Request) {
	kinds := CheckKinds()
	views := make([]checkKindView, len(kinds))
	for i, k := range kinds {
		views[i] = checkKindView{
			Kind: k.Kind, Category: k.Category, Label: k.Label, Description: k.Description,
			Values: boundsView(k.Values), Children: boundsView(k.Children), AtLeast: k.AtLeast,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"kinds": views, "max_nodes": MaxCheckNodes, "max_depth": MaxCheckDepth,
	})
}
