package middleware

import (
	"encoding/json"
	"net/http"

	"ozon-api-separator/internal/config"
)

// AuthMiddleware - middleware для проверки авторизации
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Auth-Token")

		// Если токен не передан - сразу 401
		if token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		// Проверяем токен
		if config.AppConfig.AuthToken != "" && token == config.AppConfig.AuthToken {
			next(w, r)
			return
		}

		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}
}
