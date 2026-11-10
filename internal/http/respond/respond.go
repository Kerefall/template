// Package respond - общие помощники для ответов хендлеров.
package respond

import (
	"encoding/json"
	"net/http"
)

// JSON пишет значение как application/json с заданным статусом.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
