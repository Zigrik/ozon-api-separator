package middleware

import (
	"encoding/json"
	"net/http"

	"ozon-api-separator/internal/config"
)

func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Auth-Token")
		if token != "" && config.AppConfig.AuthToken != "" && token == config.AppConfig.AuthToken {
			next(w, r)
			return
		}
		password := r.Header.Get("X-Password")
		if password == config.AppConfig.Password {
			next(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}
}
