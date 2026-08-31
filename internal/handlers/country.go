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
		log.Printf("[ERROR] Кабинет '%s' не настроен", cabinet.Name)
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	countries, err := services.GetCountriesList(cabinet)
	if err != nil {
		log.Printf("[ERROR] Ошибка получения списка стран: %v", err)
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
		log.Printf("[ERROR] Ошибка декодирования запроса: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("[ERROR] Кабинет '%s' не настроен", cabinet.Name)
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	// Если product_id = 0 - ищем правильный
	finalProductID := req.ProductID
	if finalProductID == 0 {
		log.Printf("[DEBUG] Поиск product_id для заказа %s", req.PostingNumber)

		// Получаем заказы из состояния
		state, err := services.LoadCabinetState(cabinet.Key)
		if err == nil {
			for _, order := range state.Orders {
				if order.PostingNumber == req.PostingNumber {
					for _, product := range order.Products {
						// Ищем товар, которому требуется страна
						if product.Requirements.IsCountryRequired {
							// Используем product_id или sku
							if product.ProductID != 0 {
								finalProductID = product.ProductID
							} else if product.SKU != 0 {
								finalProductID = product.SKU
							}
							log.Printf("[INFO] Найден product_id=%d (sku=%d) для заказа %s", finalProductID, product.SKU, req.PostingNumber)
							break
						}
					}
					break
				}
			}
		}

		// Если не нашли в состоянии - пробуем получить из API
		if finalProductID == 0 {
			orders, err := services.GetAwaitingPackagingOrders(cabinet, []int64{})
			if err == nil {
				for _, order := range orders {
					if order.PostingNumber == req.PostingNumber {
						for _, product := range order.Products {
							if product.IsCountryRequired {
								if product.ProductID != 0 {
									finalProductID = product.ProductID
								} else if product.SKU != 0 {
									finalProductID = product.SKU
								}
								log.Printf("[INFO] Найден product_id=%d из API для заказа %s", finalProductID, req.PostingNumber)
								break
							}
						}
						break
					}
				}
			}
		}
	}

	if finalProductID == 0 {
		log.Printf("[ERROR] Не удалось найти product_id для заказа %s", req.PostingNumber)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": "Не удалось определить ID товара",
		})
		return
	}

	log.Printf("[INFO] Установка страны %s для товара %d в заказе %s", req.CountryCode, finalProductID, req.PostingNumber)

	if err := services.SetCountry(cabinet, req.PostingNumber, finalProductID, req.CountryCode); err != nil {
		log.Printf("[ERROR] Ошибка установки страны: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	// Если для товара требуется ГТД - отмечаем его как отсутствующее
	if err := services.SetGTDAsAbsent(cabinet, req.PostingNumber, finalProductID); err != nil {
		log.Printf("[WARNING] Ошибка отметки ГТД как отсутствующего: %v", err)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": "Страна производителя установлена, ГТД отмечено как отсутствующее",
	})
}
