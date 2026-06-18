package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

// HandleGetCountries - обработчик получения списка стран
func HandleGetCountries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("❌ Кабинет '%s' не настроен", cabinet.Name)
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	countries, err := services.GetCountriesList(cabinet)
	if err != nil {
		log.Printf("❌ Ошибка получения списка стран: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"countries": countries,
	})
}

// HandleSetCountry - обработчик установки страны производителя
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

	log.Printf("🌍 Установка страны %s для товара %d в заказе %s", req.CountryCode, req.ProductID, req.PostingNumber)

	if err := services.SetCountry(cabinet, req.PostingNumber, req.ProductID, req.CountryCode); err != nil {
		log.Printf("❌ Ошибка установки страны: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	// Если для товара требуется ГТД - отмечаем его как отсутствующее
	if err := services.SetGTDAsAbsent(cabinet, req.PostingNumber, req.ProductID); err != nil {
		log.Printf("⚠️ Ошибка отметки ГТД как отсутствующего: %v", err)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": "Страна производителя установлена, ГТД отмечено как отсутствующее",
	})
}
