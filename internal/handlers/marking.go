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

	log.Printf("[INFO] Запрос количества кодов: %d", count)

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
		log.Printf("[ERROR] Ошибка перезагрузки кодов: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	config.CodesMutex.Lock()
	count := len(config.MarkingCodes)
	config.CodesMutex.Unlock()

	log.Printf("[INFO] Коды маркировки перезагружены: %d", count)

	if count > 0 {
		config.CodesMutex.Lock()
		sample := config.MarkingCodes
		config.CodesMutex.Unlock()
		if len(sample) > 5 {
			sample = sample[:5]
		}
		log.Printf("[DEBUG] Пример кодов: %v", sample)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"count":   count,
		"message": fmt.Sprintf("Загружено %d кодов", count),
	})
}

// HandleAddMarkings - обработчик добавления маркировки
func HandleAddMarkings(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] HandleAddMarkings вызван")

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
		log.Printf("[ERROR] Ошибка декодирования запроса: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[INFO] Запрос на добавление маркировки: posting=%s, product_id=%d, offer_id=%s, qty=%d",
		req.PostingNumber, req.ProductID, req.OfferID, req.Quantity)

	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("[ERROR] Кабинет '%s' не настроен", cabinet.Name)
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	// Если product_id = 0 или похож на SKU (большое число), ищем правильный
	finalProductID := req.ProductID
	if finalProductID == 0 || finalProductID > 1000000000 {
		log.Printf("[DEBUG] product_id = %d, ищем правильный ID...", finalProductID)

		// Сначала ищем в состоянии
		state, err := services.LoadCabinetState(cabinet.Key)
		if err == nil {
			for _, order := range state.Orders {
				if order.PostingNumber == req.PostingNumber {
					for _, product := range order.Products {
						if product.OfferID == req.OfferID || product.SKU == req.ProductID {
							if product.ProductID != 0 {
								finalProductID = product.ProductID
								log.Printf("[INFO] Найден правильный product_id: %d", finalProductID)
							} else if product.SKU != 0 {
								finalProductID = product.SKU
								log.Printf("[INFO] product_id = 0, используем SKU: %d", finalProductID)
							}
							break
						}
					}
					break
				}
			}
		}

		// Если не нашли - ищем в API
		if finalProductID == 0 {
			orders, err := services.GetAwaitingPackagingOrders(cabinet, []int64{})
			if err == nil {
				for _, order := range orders {
					if order.PostingNumber == req.PostingNumber {
						for _, product := range order.Products {
							if product.OfferID == req.OfferID || product.SKU == req.ProductID {
								if product.ProductID != 0 {
									finalProductID = product.ProductID
									log.Printf("[INFO] Найден product_id из API: %d", finalProductID)
								} else if product.SKU != 0 {
									finalProductID = product.SKU
									log.Printf("[INFO] Используем SKU из API: %d", finalProductID)
								}
								break
							}
						}
						break
					}
				}
			}
		}
	}

	if finalProductID <= 0 {
		log.Printf("[ERROR] Невалидный ProductID: %d, OfferID: %s", finalProductID, req.OfferID)
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

	log.Printf("[INFO] Добавление маркировки для товара %d (offer_id: %s) в заказе %s (количество: %d)",
		finalProductID, req.OfferID, req.PostingNumber, req.Quantity)

	if err := services.AddMarkingsForOrder(cabinet, req.PostingNumber, finalProductID, req.Quantity, req.Codes); err != nil {
		log.Printf("[ERROR] Ошибка добавления маркировки: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	// Обновляем состояние маркировки в JSON
	if err := services.UpdateMarkingStatus(cabinet.Key, req.PostingNumber, finalProductID, req.Codes); err != nil {
		log.Printf("[WARNING] Ошибка обновления состояния маркировки: %v", err)
	}

	response := map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Добавлено %d кодов маркировки", req.Quantity),
	}
	log.Printf("[INFO] Ответ отправлен: %v", response)
	json.NewEncoder(w).Encode(response)
}
