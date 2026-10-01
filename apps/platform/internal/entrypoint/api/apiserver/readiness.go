package apiserver

import (
	"context"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

type readinessResponse struct {
	Ready        bool          `json:"ready"`
	Capabilities []envx.Status `json:"capabilities"`
	Detail       string        `json:"detail,omitempty"`
}

const readinessReuse = 2 * time.Second

type sharedReadiness struct {
	reg   *envx.Registry
	mu    sync.Mutex
	rows  []envx.Status
	taken time.Time
}

func (s *sharedReadiness) report(ctx context.Context) []envx.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rows == nil || time.Since(s.taken) >= readinessReuse {
		s.rows, s.taken = s.reg.Report(ctx, os.Getenv), time.Now()
	}
	return slices.Clone(s.rows)
}

func readinessHandler(d Deps) http.HandlerFunc {
	reg := d.Readiness
	shared := &sharedReadiness{reg: reg}
	return func(w http.ResponseWriter, r *http.Request) {
		if reg == nil {

			httpx.WriteJSON(w, http.StatusOK, readinessResponse{
				Ready: false, Capabilities: []envx.Status{}, Detail: "這個 build 沒有能力表，所以這裡量不到任何東西。",
			})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		rows := shared.report(ctx)
		if !d.CleanMode {
			for i := range rows {
				rows[i].Missing = nil
				rows[i].Detail = ""
				rows[i].Without = ""
				rows[i].Fix = ""
			}
		}
		httpx.WriteJSON(w, http.StatusOK, readinessResponse{
			Ready: envx.AllReady(rows), Capabilities: rows,
		})
	}
}
