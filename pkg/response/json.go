// Package response содержит хелперы для формирования HTTP-ответов
// в едином формате по всем сервисам платформы.
package response

import (
	"encoding/json"
	"log"
	"net/http"
)

// WriteJSON сериализует v в JSON и пишет в ResponseWriter с указанным
// HTTP-статусом. Ошибку кодирования логируем — на этом этапе заголовки
// уже отправлены, корректно вернуть её клиенту нельзя.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("response: ошибка кодирования JSON: %v", err)
	}
}

// Error отправляет ошибку в едином JSON-формате {"error": "..."}.
func Error(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}
