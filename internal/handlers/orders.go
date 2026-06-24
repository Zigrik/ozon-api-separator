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

	// Сохраняем состояние в файл (синхронно, чтобы получить is_ready_for_split)
	if err := services.UpdateOrders(cabinet.Key, cabinet.Name, orders); err != nil {
		log.Printf("⚠️ Ошибка сохранения состояния: %v", err)
	}

	// Загружаем состояние чтобы получить is_ready_for_split
	state, err := services.LoadCabinetState(cabinet.Key)
	if err != nil {
		log.Printf("⚠️ Ошибка загрузки состояния: %v", err)
	}

	// Создаем карту is_ready_for_split по номеру заказа
	readyMap := make(map[string]bool)
	if state != nil {
		for _, orderState := range state.Orders {
			readyMap[orderState.PostingNumber] = orderState.IsReadyForSplit
		}
	}

	// Обогащаем заказы данными из состояния
	for i := range orders {
		if isReady, exists := readyMap[orders[i].PostingNumber]; exists {
			orders[i].IsReadyForSplit = isReady
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"orders":  orders,
		"cabinet": cabinet.Name,
	})
}

// HandleShipOrders - обработчик разделения заказов (БЕЗ этикеток)
func HandleShipOrders(w http.ResponseWriter, r *http.Request) {
	shipOrders(w, r, 1, false)
}

// HandleShipAndOrderLabels - обработчик разделения заказов С заказом этикеток
func HandleShipAndOrderLabels(w http.ResponseWriter, r *http.Request) {
	shipOrders(w, r, 1, true)
}

// shipOrders - общая функция для разделения заказов
func shipOrders(w http.ResponseWriter, r *http.Request, needLabels int, wakeWorker bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Orders []struct {
			PostingNumber string `json:"posting_number"`
			Products      []struct {
				ProductID int64  `json:"product_id"`
				OfferID   string `json:"offer_id"`
				Quantity  int    `json:"quantity"`
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

	if needLabels == 1 && wakeWorker {
		log.Printf("📦 Начало разделения и заказа этикеток для %d заказов", len(req.Orders))
	} else {
		log.Printf("📦 Начало разделения %d заказов", len(req.Orders))
	}

	orders, err := services.GetAwaitingPackagingOrders(cabinet)
	if err != nil {
		log.Printf("⚠️ Ошибка получения заказов для исправления product_id: %v", err)
	}
	ordersMap := make(map[string][]models.Product)
	for _, order := range orders {
		ordersMap[order.PostingNumber] = order.Products
	}

	for _, orderReq := range req.Orders {
		result := map[string]interface{}{
			"posting_number": orderReq.PostingNumber,
		}

		packages := make([]models.ShipPackage, 0)
		productIDs := make([]int64, 0)

		for _, product := range orderReq.Products {
			productID := product.ProductID

			if productID == 0 {
				if products, exists := ordersMap[orderReq.PostingNumber]; exists {
					for _, p := range products {
						if p.OfferID == product.OfferID {
							productID = p.ProductID
							if productID == 0 {
								productID = p.SKU
							}
							break
						}
					}
					if productID == 0 && product.OfferID == "" {
						for _, p := range products {
							if p.SKU == product.ProductID {
								productID = p.ProductID
								if productID == 0 {
									productID = p.SKU
								}
								break
							}
						}
					}
				}
				if productID == 0 {
					log.Printf("❌ Не удалось исправить ProductID=0 для заказа %s (offer_id=%s)", orderReq.PostingNumber, product.OfferID)
					result["status"] = "error"
					result["error"] = fmt.Sprintf("Не удалось определить товар для заказа %s", orderReq.PostingNumber)
					results = append(results, result)
					continue
				} else {
					log.Printf("⚠️ ProductID был 0, исправлен на %d для заказа %s (offer_id=%s)", productID, orderReq.PostingNumber, product.OfferID)
				}
			}

			productIDs = append(productIDs, productID)
			for i := 0; i < product.Quantity; i++ {
				packages = append(packages, models.ShipPackage{
					Products: []models.ShipProduct{
						{
							ProductID: productID,
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

		shipments, err := services.ShipOrder(cabinet, orderReq.PostingNumber, packages)
		if err != nil {
			log.Printf("❌ Ошибка разделения заказа %s: %v", orderReq.PostingNumber, err)
			result["status"] = "error"
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}

		log.Printf("✅ Заказ %s разделён на %d отправлений", orderReq.PostingNumber, len(shipments))

		if err := services.UpdateOrderAfterShip(cabinet.Key, orderReq.PostingNumber, shipments, productIDs, needLabels); err != nil {
			log.Printf("⚠️ Ошибка сохранения состояния после разделения: %v", err)
		}

		if wakeWorker {
			services.WakeLabelWorker()
			log.Printf("🔔 Пробужден воркер заказа этикеток для заказа %s", orderReq.PostingNumber)
		} else {
			log.Printf("📌 Заказ %s разделен, этикетки будут заказаны позже (labels_status=1)", orderReq.PostingNumber)
		}

		result["status"] = "success"
		result["shipments"] = shipments

		if wakeWorker {
			result["message"] = fmt.Sprintf("Заказ разделён на %d отправлений, этикетки будут заказаны автоматически", len(shipments))
		} else {
			result["message"] = fmt.Sprintf("Заказ разделён на %d отправлений (этикетки не заказаны)", len(shipments))
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

	if wakeWorker {
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

// HandleGetStats - обработчик получения статистики по кабинету
func HandleGetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cabinetKey := r.URL.Query().Get("cabinet")
	if cabinetKey == "" {
		http.Error(w, "cabinet is required", http.StatusBadRequest)
		return
	}

	if _, exists := config.AppConfig.Cabinets[cabinetKey]; !exists {
		http.Error(w, "Cabinet not found", http.StatusNotFound)
		return
	}

	state, err := services.LoadCabinetState(cabinetKey)
	if err != nil {
		log.Printf("❌ Ошибка загрузки состояния: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	total := 0
	divided := 0
	toOrder := 0
	toDownload := 0

	for _, order := range state.Orders {
		orderTotal := 0
		for _, product := range order.Products {
			orderTotal += product.Quantity
		}
		total += orderTotal

		if order.IsDivided {
			divided += orderTotal
		}

		if order.IsDivided {
			switch order.LabelsStatus {
			case 1:
				toOrder += orderTotal
			case 2:
				toDownload += orderTotal
			}
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"total":       total,
		"divided":     divided,
		"to_order":    toOrder,
		"to_download": toDownload,
	})
}
