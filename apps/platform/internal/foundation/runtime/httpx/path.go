package httpx

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
)

func PathUUID(w http.ResponseWriter, r *http.Request, name, notFound string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(r.PathValue(name)); err != nil {
		WriteError(w, http.StatusNotFound, notFound)
		return pgtype.UUID{}, false
	}
	return id, true
}
