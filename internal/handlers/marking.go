package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

// HandleAddMarkings - обработчик добавления маркировки из файла
func HandleAddMarkings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PostingNumber string   `json:"posting_number"`
		ProductID     int64    `json:"product_id"`
		Quantity      int      `json:"quantity"`
		Codes         []string `json:"codes"`
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

	if len(req.Codes) < req.Quantity {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Недостаточно кодов маркировки: нужно %d, получено %d", req.Quantity, len(req.Codes)),
		})
		return
	}

	log.Printf("🏷️ Добавление маркировки для товара %d в заказе %s (количество: %d)", req.ProductID, req.PostingNumber, req.Quantity)

	if err := services.AddMarkingsForOrder(cabinet, req.PostingNumber, req.ProductID, req.Quantity, req.Codes); err != nil {
		log.Printf("❌ Ошибка добавления маркировки: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Добавлено %d кодов маркировки", req.Quantity),
	})
}
