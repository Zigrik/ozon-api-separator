package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/models"
	"ozon-api-separator/internal/services"
)

// HandleGetOrders - обработчик получения списка заказов
// Метод: GET
// Возвращает заказы с информацией о требованиях (маркировка, ГТД, страна)
func HandleGetOrders(w http.ResponseWriter, r *http.Request) {
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

	log.Printf("📦 Загрузка заказов для кабинета '%s'", cabinet.Name)

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

// HandleShipOrders - обработчик разделения заказов
// Метод: POST
// Тело запроса: {"orders": [{"posting_number": "...", "products": [...]}]}
// Возвращает результаты разделения для каждого заказа
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
		log.Printf("❌ Ошибка декодирования запроса: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cabinet := config.GetActiveConfig()
	results := make([]map[string]interface{}, 0)

	log.Printf("📦 Начало разделения %d заказов", len(req.Orders))

	for _, orderReq := range req.Orders {
		result := map[string]interface{}{
			"posting_number": orderReq.PostingNumber,
		}

		// Формируем упаковки: каждый товар отдельно (поштучно)
		packages := make([]models.ShipPackage, 0)
		for _, product := range orderReq.Products {
			for i := 0; i < product.Quantity; i++ {
				packages = append(packages, models.ShipPackage{
					Products: []models.ShipProduct{
						{
							ProductID: product.ProductID,
							Quantity:  1,
						},
					},
				})
			}
		}

		if len(packages) == 0 {
			result["status"] = "error"
			result["error"] = "Нет товаров для отправки"
			results = append(results, result)
			continue
		}

		// Выполняем разделение заказа
		shipments, err := services.ShipOrder(cabinet, orderReq.PostingNumber, packages)
		if err != nil {
			log.Printf("❌ Ошибка разделения заказа %s: %v", orderReq.PostingNumber, err)
			result["status"] = "error"
			result["error"] = err.Error()
		} else {
			log.Printf("✅ Заказ %s разделён на %d отправлений", orderReq.PostingNumber, len(shipments))
			result["status"] = "success"
			result["shipments"] = shipments
			result["message"] = fmt.Sprintf("Заказ разделён на %d отправлений", len(shipments))
		}
		results = append(results, result)
	}

	// Подсчитываем статистику
	successCount := 0
	errorCount := 0
	for _, r := range results {
		if r["status"] == "success" {
			successCount++
		} else {
			errorCount++
		}
	}
	log.Printf("📊 Разделение завершено: успешно %d, ошибок %d", successCount, errorCount)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"results": results,
	})
}
