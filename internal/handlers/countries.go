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

	cabinet := config.GetActiveConfig()

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
					if product.ProductID == req.ProductID && product.IsGtdRequired {
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
