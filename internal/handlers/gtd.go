package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

// HandleSetGTDAbsent - обработчик отметки ГТД как отсутствующего
func HandleSetGTDAbsent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PostingNumber string `json:"posting_number"`
		ProductID     int64  `json:"product_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Ошибка декодирования запроса: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("❌ Кабинет '%s' не настроен", cabinet.Name)
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	log.Printf("📄 Отметка ГТД как отсутствующего для товара %d в заказе %s", req.ProductID, req.PostingNumber)

	if err := services.SetGTDAsAbsent(cabinet, req.PostingNumber, req.ProductID); err != nil {
		log.Printf("❌ Ошибка отметки ГТД: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": "ГТД отмечено как отсутствующее",
	})
}
