package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

func HandleGetCountries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}
	countries, err := services.GetCountriesList(cabinet)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"countries": countries,
	})
}

func HandleSetCountry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PostingNumber string `json:"posting_number"`
		ProductID     int64  `json:"product_id"`
		CountryCode   string `json:"country_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Ошибка декодирования: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("📥 Запрос на установку страны: posting=%s, product_id=%d, country=%s",
		req.PostingNumber, req.ProductID, req.CountryCode)

	cabinet := config.GetActiveConfig()

	// Если product_id == 0, пытаемся найти правильный ID
	if req.ProductID == 0 {
		log.Printf("⚠️ product_id = 0, пытаемся найти правильный ID...")

		// Получаем заказы
		orders, err := services.GetAwaitingPackagingOrders(cabinet)
		if err == nil {
			for _, order := range orders {
				if order.PostingNumber == req.PostingNumber {
					for _, product := range order.Products {
						// Используем SKU если ProductID = 0
						if product.ProductID == 0 && product.SKU != 0 {
							req.ProductID = product.SKU
							log.Printf("🔧 Исправлен product_id: используем SKU = %d", req.ProductID)
							break
						}
						// Если product.SKU совпадает с тем, что прислали как product_id
						if product.SKU == req.ProductID {
							req.ProductID = product.ProductID
							if req.ProductID == 0 {
								req.ProductID = product.SKU
							}
							log.Printf("🔧 Исправлен product_id: найден по SKU = %d", req.ProductID)
							break
						}
					}
					break
				}
			}
		}
	}

	// Последняя проверка
	if req.ProductID <= 0 {
		errMsg := fmt.Sprintf("Невалидный ProductID: %d. Невозможно установить страну производителя.", req.ProductID)
		log.Printf("❌ %s", errMsg)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	// Устанавливаем страну производителя
	err := services.SetCountry(cabinet, req.PostingNumber, req.ProductID, req.CountryCode)
	if err != nil {
		log.Printf("❌ Ошибка установки страны: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	log.Printf("✅ Страна производителя установлена: %s для заказа %s, товар %d",
		req.CountryCode, req.PostingNumber, req.ProductID)

	// Если для этого товара требуется ГТД - отмечаем его как отсутствующее
	orders, err := services.GetAwaitingPackagingOrders(cabinet)
	if err == nil {
		prefix := req.PostingNumber[:len(req.PostingNumber)-2] // убираем "-1" в конце
		for _, order := range orders {
			if order.PostingNumber == req.PostingNumber || strings.HasPrefix(order.PostingNumber, prefix) {
				for _, product := range order.Products {
					// Сравниваем и по ProductID, и по SKU
					productIdMatch := product.ProductID == req.ProductID
					skuMatch := product.SKU == req.ProductID

					if (productIdMatch || skuMatch) && product.IsGtdRequired {
						// Отмечаем ГТД как отсутствующее
						err := services.SetGTDAsAbsent(cabinet, req.PostingNumber, req.ProductID)
						if err != nil {
							log.Printf("⚠️ Ошибка отметки ГТД как отсутствующего для товара %d: %v", req.ProductID, err)
						} else {
							log.Printf("✅ ГТД отмечено как отсутствующее для товара %d в заказе %s", req.ProductID, req.PostingNumber)
						}
						break
					}
				}
				break
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Страна производителя установлена: %s", req.CountryCode),
	})
}
