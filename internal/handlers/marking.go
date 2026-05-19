package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

// HandleAddMarkingsWithGTD — добавляет маркировку (КИЗ) и отмечает ГТД как отсутствующее
func HandleAddMarkingsWithGTD(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PostingNumber string `json:"posting_number"`
		ProductID     int64  `json:"product_id"`
		Quantity      int    `json:"quantity"`
		GTDAbsent     bool   `json:"gtd_absent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Ошибка декодирования запроса: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("📥 Запрос на добавление маркировки: posting=%s, product_id=%d, qty=%d",
		req.PostingNumber, req.ProductID, req.Quantity)

	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("❌ Кабинет не настроен")
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	// Если product_id == 0, пытаемся найти правильный ID
	if req.ProductID == 0 {
		log.Printf("⚠️ product_id = 0, пытаемся найти правильный ID...")

		orders, err := services.GetAwaitingPackagingOrders(cabinet)
		if err == nil {
			for _, order := range orders {
				if order.PostingNumber == req.PostingNumber {
					for _, product := range order.Products {
						if product.SKU != 0 {
							req.ProductID = product.SKU
							log.Printf("🔧 Исправлен product_id для маркировки: используем SKU = %d", req.ProductID)
							break
						}
					}
					break
				}
			}
		}
	}

	if req.ProductID <= 0 {
		errMsg := fmt.Sprintf("Невалидный ProductID: %d. Невозможно добавить маркировку.", req.ProductID)
		log.Printf("❌ %s", errMsg)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	codes, err := config.GetMarkingCodes(req.Quantity)
	if err != nil {
		log.Printf("❌ Ошибка получения кодов маркировки: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	if err := services.AddMarkingsForOrder(cabinet, req.PostingNumber, req.ProductID, req.Quantity, codes); err != nil {
		log.Printf("❌ Ошибка добавления маркировки: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	log.Printf("✅ Успешно добавлено %d кодов маркировки для заказа %s", req.Quantity, req.PostingNumber)

	config.CodesMutex.Lock()
	remaining := len(config.MarkingCodes)
	config.CodesMutex.Unlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"message":   fmt.Sprintf("Добавлено %d кодов маркировки, ГТД отмечено как отсутствующее", req.Quantity),
		"remaining": remaining,
	})
}
