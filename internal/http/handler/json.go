package handler

import (
	"encoding/json"
	"net/http"
)

// writeJSON escreve uma resposta JSON com status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
