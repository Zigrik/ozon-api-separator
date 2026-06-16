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

	log.Printf("✅ Загружено %d заказов из Ozon API", len(orders))

	// Сохраняем состояние в файл (в фоне)
	go func() {
		if err := services.UpdateOrders(cabinet.Key, cabinet.Name, orders); err != nil {
			log.Printf("⚠️ Ошибка сохранения состояния: %v", err)
		}
	}()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"orders":  orders,
		"cabinet": cabinet.Name,
	})
}

// HandleShipOrders - обработчик разделения заказов (без этикеток)
func HandleShipOrders(w http.ResponseWriter, r *http.Request) {
	shipOrders(w, r, 0) // labels_status = 0 (этикетки не нужны)
}

// HandleShipAndOrderLabels - обработчик разделения заказов с заказом этикеток
func HandleShipAndOrderLabels(w http.ResponseWriter, r *http.Request) {
	shipOrders(w, r, 1) // labels_status = 1 (этикетки заказаны)
}

// shipOrders - общая функция для разделения заказов
// needLabels:
//
//	0 - только разделение (labels_status = 0)
//	1 - разделение + заказ этикеток (labels_status = 1)
func shipOrders(w http.ResponseWriter, r *http.Request, needLabels int) {
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

	if needLabels == 1 {
		log.Printf("📦 Начало разделения и заказа этикеток для %d заказов", len(req.Orders))
	} else {
		log.Printf("📦 Начало разделения %d заказов", len(req.Orders))
	}

	for _, orderReq := range req.Orders {
		result := map[string]interface{}{
			"posting_number": orderReq.PostingNumber,
		}

		// 1. Формируем упаковки
		packages := make([]models.ShipPackage, 0)
		productIDs := make([]int64, 0)

		for _, product := range orderReq.Products {
			productIDs = append(productIDs, product.ProductID)
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

		// 2. Разделяем заказ
		shipments, err := services.ShipOrder(cabinet, orderReq.PostingNumber, packages)
		if err != nil {
			log.Printf("❌ Ошибка разделения заказа %s: %v", orderReq.PostingNumber, err)
			result["status"] = "error"
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}

		log.Printf("✅ Заказ %s разделён на %d отправлений", orderReq.PostingNumber, len(shipments))

		// 3. Обновляем состояние: is_divided = true, labels_status = needLabels
		if err := services.UpdateOrderAfterShip(cabinet.Key, orderReq.PostingNumber, shipments, productIDs, needLabels); err != nil {
			log.Printf("⚠️ Ошибка сохранения состояния после разделения: %v", err)
		}

		// 4. Если needLabels = 1 - заказываем этикетки для каждого подзаказа
		var labelResults []map[string]interface{}
		if needLabels == 1 {
			labelResults = make([]map[string]interface{}, 0)
			for _, shipment := range shipments {
				labelResult := map[string]interface{}{
					"posting_number": shipment,
				}

				taskID, err := services.CreateLabelTask(cabinet, []string{shipment})
				if err != nil {
					log.Printf("❌ Ошибка заказа этикетки для %s: %v", shipment, err)
					labelResult["status"] = "error"
					labelResult["error"] = err.Error()
				} else {
					log.Printf("🏷️ Этикетка для %s заказана, task_id: %d", shipment, taskID)

					if err := services.UpdateOrderLabel(cabinet.Key, shipment, taskID, true, ""); err != nil {
						log.Printf("⚠️ Ошибка сохранения состояния этикетки: %v", err)
					}

					labelResult["status"] = "success"
					labelResult["task_id"] = taskID
				}
				labelResults = append(labelResults, labelResult)
			}
		}

		// 5. Формируем результат
		result["status"] = "success"
		result["shipments"] = shipments

		if needLabels == 1 {
			result["labels"] = labelResults
			result["message"] = fmt.Sprintf("Заказ разделён на %d отправлений, этикетки заказаны", len(shipments))
		} else {
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

	if needLabels == 1 {
		log.Printf("📊 Разделение и заказ этикеток завершены: успешно %d, ошибок %d", successCount, errorCount)
	} else {
		log.Printf("📊 Разделение завершено: успешно %d, ошибок %d", successCount, errorCount)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"results": results,
	})
}

// HandleGetOrderState - получить состояние конкретного заказа из файла
func HandleGetOrderState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	postingNumber := r.URL.Query().Get("posting_number")
	if postingNumber == "" {
		http.Error(w, "posting_number is required", http.StatusBadRequest)
		return
	}

	cabinet := config.GetActiveConfig()

	orderState, err := services.GetOrderState(cabinet.Key, postingNumber)
	if err != nil {
		log.Printf("❌ Ошибка получения состояния заказа: %v", err)
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"order":  orderState,
	})
}
