package handlers

import (
	"encoding/json"
	"net/http"

	"ozon-api-separator/internal/config"
)

// HandleCheckPassword - обработчик проверки пароля
// Метод: POST
// Тело запроса: {"password": "string"}
// Ответ при успехе: {"status": "ok", "token": "string"}
// Ответ при ошибке: 401 Unauthorized + {"error": "invalid password"}
func HandleCheckPassword(w http.ResponseWriter, r *http.Request) {
	// Проверяем метод запроса
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Декодируем тело запроса
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Проверяем пароль
	if req.Password == config.AppConfig.Password {
		// Пароль верный - возвращаем токен для последующих запросов
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"token":  config.AppConfig.AuthToken,
		})
	} else {
		// Пароль неверный
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid password"})
	}
}
