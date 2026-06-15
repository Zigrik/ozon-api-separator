package middleware

import (
	"encoding/json"
	"net/http"

	"ozon-api-separator/internal/config"
)

// AuthMiddleware - middleware для проверки авторизации
// Проверяет наличие валидного токена в заголовке X-Auth-Token
// Возвращает 401 Unauthorized, если токен невалиден
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Получаем токен из заголовка
		token := r.Header.Get("X-Auth-Token")

		// Проверяем токен (если он установлен в конфиге)
		if token != "" && config.AppConfig.AuthToken != "" && token == config.AppConfig.AuthToken {
			next(w, r) // Токен валиден - передаем управление дальше
			return
		}

		// Токен невалиден или отсутствует
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}
}
