package apiserver

import (
	"net/http"

	publicapi "github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

func newGeneratedHealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, publicapi.Health{Status: publicapi.HealthStatusOk})
	})
}
