package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

// HandleGetAvailableCodes - получение количества доступных кодов маркировки
func HandleGetAvailableCodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config.CodesMutex.Lock()
	count := len(config.MarkingCodes)
	config.CodesMutex.Unlock()

	log.Printf("📊 Запрос количества кодов: %d", count)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"count":  count,
	})
}

// HandleGetCodes - получение кодов маркировки из файла
func HandleGetCodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Count <= 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": "Количество должно быть больше 0",
		})
		return
	}

	codes, err := config.GetMarkingCodes(req.Count)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"codes":  codes,
	})
}

// HandleReloadCodes - перезагрузка кодов маркировки из файла
func HandleReloadCodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := config.LoadMarkingCodes(); err != nil {
		log.Printf("❌ Ошибка перезагрузки кодов: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	config.CodesMutex.Lock()
	count := len(config.MarkingCodes)
	config.CodesMutex.Unlock()

	log.Printf("✅ Коды маркировки перезагружены: %d", count)

	if count > 0 {
		config.CodesMutex.Lock()
		sample := config.MarkingCodes
		config.CodesMutex.Unlock()
		if len(sample) > 5 {
			sample = sample[:5]
		}
		log.Printf("📋 Пример кодов: %v", sample)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"count":   count,
		"message": fmt.Sprintf("Загружено %d кодов", count),
	})
}

func HandleAddMarkings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PostingNumber string   `json:"posting_number"`
		ProductID     int64    `json:"product_id"`
		OfferID       string   `json:"offer_id"`
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

	// Если product_id = 0, пытаемся найти по offer_id
	if req.ProductID == 0 && req.OfferID != "" {
		orders, err := services.GetAwaitingPackagingOrders(cabinet)
		if err == nil {
			for _, order := range orders {
				if order.PostingNumber == req.PostingNumber {
					for _, p := range order.Products {
						if p.OfferID == req.OfferID {
							req.ProductID = p.ProductID
							if req.ProductID == 0 {
								req.ProductID = p.SKU
							}
							log.Printf("🔧 Найден product_id по offer_id: %d", req.ProductID)
							break
						}
					}
					break
				}
			}
		}
	}

	if req.ProductID <= 0 {
		log.Printf("❌ Невалидный ProductID: %d", req.ProductID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": "Не удалось определить ID товара",
		})
		return
	}

	if len(req.Codes) < req.Quantity {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Недостаточно кодов маркировки: нужно %d, получено %d", req.Quantity, len(req.Codes)),
		})
		return
	}

	log.Printf("🏷️ Добавление маркировки для товара %d (offer_id: %s) в заказе %s (количество: %d)",
		req.ProductID, req.OfferID, req.PostingNumber, req.Quantity)

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
