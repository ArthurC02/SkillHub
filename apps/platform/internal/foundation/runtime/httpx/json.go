package httpx

import (
	"encoding/json"
	"net/http"
)

func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, code int, msg string) {
	WriteJSON(w, code, map[string]string{"error": msg})
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, v any, badBody string) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBytes)).Decode(v); err != nil {
		WriteError(w, http.StatusBadRequest, badBody)
		return false
	}
	return true
}
