package handlers

import (
	"encoding/json"
	"net/http"

	"ozon-api-separator/internal/config"
)

// HandleSwitchCabinet - обработчик переключения активного кабинета
// Метод: POST
// Тело запроса: {"cabinet": "shinorama"}
// Ответ: {"status": "ok", "active": "shinorama"}
func HandleSwitchCabinet(w http.ResponseWriter, r *http.Request) {
	// Проверяем метод запроса
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Декодируем тело запроса
	var req struct {
		Cabinet string `json:"cabinet"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Проверяем, существует ли запрошенный кабинет
	if _, exists := config.AppConfig.Cabinets[req.Cabinet]; !exists {
		http.Error(w, "Cabinet not found", http.StatusNotFound)
		return
	}

	// Переключаем активный кабинет
	config.AppConfig.ActiveCabinet = req.Cabinet

	// Возвращаем успешный ответ
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"active": req.Cabinet,
	})
}
