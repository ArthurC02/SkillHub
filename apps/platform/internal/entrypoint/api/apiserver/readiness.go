package apiserver

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

func readinessHandler(reg *envx.Registry, clean bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if reg == nil {

			httpx.WriteJSON(w, http.StatusOK, map[string]any{
				"ready":        false,
				"capabilities": []envx.Status{},
				"detail":       "這個 build 沒有能力表，所以這裡量不到任何東西。",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		rows := reg.Report(ctx, os.Getenv)
		if !clean {
			for i := range rows {
				rows[i].Missing = nil
				rows[i].Detail = ""
				rows[i].Without = ""
				rows[i].Fix = ""
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"ready":        envx.AllReady(rows),
			"capabilities": rows,
		})
	}
}
