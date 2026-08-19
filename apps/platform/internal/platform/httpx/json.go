package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes v with the given status; encoding failures are dropped
// because the status line is already gone.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes the contract's Error shape (public.yaml).
func WriteError(w http.ResponseWriter, code int, msg string) {
	WriteJSON(w, code, map[string]string{"error": msg})
}
