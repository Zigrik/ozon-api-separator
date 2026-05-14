package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

func HandleGetOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("❌ Кабинет не настроен")
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}
	log.Printf("📦 Загрузка заказов для кабинета %s", cabinet.Name)
	orders, err := services.GetAwaitingPackagingOrders(cabinet)
	if err != nil {
		log.Printf("❌ Ошибка загрузки заказов: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("✅ Загружено %d заказов", len(orders))
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"orders":  orders,
		"cabinet": cabinet.Name,
	})
}

func HandleShipOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Orders []struct {
			PostingNumber string `json:"posting_number"`
			Products      []struct {
				ProductID int64 `json:"product_id"`
				Quantity  int   `json:"quantity"`
			} `json:"products"`
		} `json:"orders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Ошибка декодирования: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cabinet := config.GetActiveConfig()
	results := make([]map[string]interface{}, 0)
	log.Printf("📦 Начало отправки %d заказов", len(req.Orders))

	for _, order := range req.Orders {
		// Проверяем и исправляем ProductID
		fixedProducts := make([]struct {
			ProductID int64
			Quantity  int
		}, 0)

		for _, product := range order.Products {
			productID := product.ProductID
			if productID == 0 {
				// Пытаемся найти правильный ProductID через получение заказа
				orders, err := services.GetAwaitingPackagingOrders(cabinet)
				if err == nil {
					for _, o := range orders {
						if o.PostingNumber == order.PostingNumber {
							for _, p := range o.Products {
								if p.SKU == product.ProductID || p.OfferID != "" {
									productID = p.ProductID
									if productID == 0 {
										productID = p.SKU
									}
									break
								}
							}
							break
						}
					}
				}
				if productID == 0 {
					productID = product.ProductID // оставляем как есть, но это вызовет ошибку
				}
				log.Printf("⚠️ ProductID был 0, исправлен на %d", productID)
			}
			fixedProducts = append(fixedProducts, struct {
				ProductID int64
				Quantity  int
			}{ProductID: productID, Quantity: product.Quantity})
		}

		packages := make([]services.ShipPackage, 0)
		for _, product := range fixedProducts {
			for i := 0; i < product.Quantity; i++ {
				packages = append(packages, services.ShipPackage{
					Products: []services.ShipProduct{
						{
							ProductID: product.ProductID,
							Quantity:  1,
						},
					},
				})
			}
		}

		result := map[string]interface{}{
			"posting_number": order.PostingNumber,
		}

		maxRetries := 3
		retryDelay := 1 * time.Second
		var shipments []string
		var err error

		for attempt := 1; attempt <= maxRetries; attempt++ {
			shipments, err = services.ShipOrder(cabinet, order.PostingNumber, packages)
			if err == nil {
				break
			}
			log.Printf("⚠️ Попытка %d/%d: ошибка отправки %s: %v", attempt, maxRetries, order.PostingNumber, err)
			if attempt < maxRetries {
				time.Sleep(retryDelay)
			}
		}

		if err != nil {
			result["status"] = "error"
			result["error"] = err.Error()
			log.Printf("❌ Ошибка отправки %s после %d попыток: %v", order.PostingNumber, maxRetries, err)
		} else {
			result["status"] = "success"
			result["shipments"] = shipments
			result["message"] = fmt.Sprintf("Заказ %s разделен на %d отправлений", order.PostingNumber, len(packages))
			log.Printf("✅ Отправлен %s на %d упаковок", order.PostingNumber, len(packages))
		}
		results = append(results, result)
	}

	successCount := 0
	errorCount := 0
	for _, r := range results {
		if r["status"] == "success" {
			successCount++
		} else {
			errorCount++
		}
	}
	log.Printf("📊 Отправка завершена: успешно %d, ошибок %d", successCount, errorCount)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"results": results,
	})
}
