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

	// Сохраняем состояние в файл
	if err := services.UpdateOrders(cabinet.Key, cabinet.Name, orders); err != nil {
		log.Printf("⚠️ Ошибка сохранения состояния: %v", err)
	}

	// Загружаем состояние чтобы получить is_ready_for_split и статус маркировки
	state, err := services.LoadCabinetState(cabinet.Key)
	if err != nil {
		log.Printf("⚠️ Ошибка загрузки состояния: %v", err)
	}

	// Создаем карты для обогащения данных
	readyMap := make(map[string]bool)
	markingMap := make(map[string]map[int64]bool)

	if state != nil {
		log.Printf("📂 Загружено состояние из JSON, заказов: %d", len(state.Orders))
		for _, orderState := range state.Orders {
			readyMap[orderState.PostingNumber] = orderState.IsReadyForSplit
			for _, product := range orderState.Products {
				if markingMap[orderState.PostingNumber] == nil {
					markingMap[orderState.PostingNumber] = make(map[int64]bool)
				}
				markingMap[orderState.PostingNumber][product.ProductID] = product.Marking.IsCompleted
				log.Printf("   JSON: заказ %s, товар %d, marking_completed=%v",
					orderState.PostingNumber, product.ProductID, product.Marking.IsCompleted)
			}
		}
	}

	// Проверяем статус маркировки через API для каждого заказа
	for i := range orders {
		order := &orders[i]
		log.Printf("🔍 Обработка заказа %s", order.PostingNumber)

		// Проверяем, есть ли товары, требующие маркировки/ГТД
		needsCheck := false
		for _, p := range order.Products {
			if p.IsMandatoryMarked || p.IsGtdRequired {
				needsCheck = true
				log.Printf("   Товар %d требует маркировки/ГТД: is_mandatory_marked=%v, is_gtd_required=%v",
					p.ProductID, p.IsMandatoryMarked, p.IsGtdRequired)
			}
		}

		if !needsCheck {
			log.Printf("   Заказ %s не требует маркировки, пропускаем", order.PostingNumber)
			continue
		}

		// Проверяем статус через API
		markingStatus, err := services.CheckMarkingStatus(cabinet, order.PostingNumber)
		if err != nil {
			log.Printf("⚠️ Ошибка проверки статуса маркировки для заказа %s: %v", order.PostingNumber, err)
			continue
		}

		log.Printf("   Статус маркировки из API для заказа %s: %v", order.PostingNumber, markingStatus)

		// Обновляем статус в JSON
		if err := services.UpdateMarkingStatusFromAPI(cabinet.Key, order.PostingNumber, markingStatus); err != nil {
			log.Printf("⚠️ Ошибка обновления статуса маркировки: %v", err)
		}

		// Обновляем товары в заказе
		for j := range order.Products {
			product := &order.Products[j]
			log.Printf("   Проверка товара %d: product_id=%d, sku=%d", j, product.ProductID, product.SKU)

			// Сначала пробуем найти по SKU (если product_id = 0)
			if product.ProductID == 0 && product.SKU != 0 {
				if isCompleted, exists := markingStatus[product.SKU]; exists && isCompleted {
					product.IsMarkingCompleted = true
					product.IsMandatoryMarked = false
					product.IsGtdRequired = false
					log.Printf("   ✅ Маркировка подтверждена для товара %d (по SKU) в заказе %s", product.SKU, order.PostingNumber)
					continue
				}
			}

			// Проверяем по product_id
			if isCompleted, exists := markingStatus[product.ProductID]; exists && isCompleted {
				product.IsMarkingCompleted = true
				product.IsMandatoryMarked = false
				product.IsGtdRequired = false
				log.Printf("   ✅ Маркировка подтверждена для товара %d (product_id) в заказе %s", product.ProductID, order.PostingNumber)
				continue
			}

			// Если все еще не нашли, пробуем по SKU (запасной вариант)
			if !product.IsMarkingCompleted && product.SKU != 0 {
				for pid, isCompleted := range markingStatus {
					if pid == product.SKU && isCompleted {
						product.IsMarkingCompleted = true
						product.IsMandatoryMarked = false
						product.IsGtdRequired = false
						log.Printf("   ✅ Маркировка подтверждена для товара %d (по SKU через цикл) в заказе %s", product.SKU, order.PostingNumber)
						break
					}
				}
			}

			if !product.IsMarkingCompleted {
				log.Printf("   ❌ Маркировка НЕ подтверждена для товара %d (product_id=%d, sku=%d)",
					j, product.ProductID, product.SKU)
			}
		}
	}

	// Дополнительно проверяем состояние из JSON
	if markingMap != nil {
		log.Printf("📂 Проверка состояния из JSON")
		for _, order := range orders {
			if markingMap[order.PostingNumber] != nil {
				for j := range order.Products {
					product := &order.Products[j]
					// Проверяем по product_id
					if isCompleted, exists := markingMap[order.PostingNumber][product.ProductID]; exists && isCompleted {
						product.IsMarkingCompleted = true
						product.IsMandatoryMarked = false
						product.IsGtdRequired = false
						log.Printf("   ✅ Из JSON: маркировка подтверждена для товара %d в заказе %s", product.ProductID, order.PostingNumber)
					}
					// Проверяем по SKU
					if !product.IsMarkingCompleted && product.SKU != 0 {
						for pid, isCompleted := range markingMap[order.PostingNumber] {
							if pid == product.SKU && isCompleted {
								product.IsMarkingCompleted = true
								product.IsMandatoryMarked = false
								product.IsGtdRequired = false
								log.Printf("   ✅ Из JSON (по SKU): маркировка подтверждена для товара %d в заказе %s", product.SKU, order.PostingNumber)
								break
							}
						}
					}
				}
			}
		}
	}

	// Обогащаем is_ready_for_split
	for i := range orders {
		if isReady, exists := readyMap[orders[i].PostingNumber]; exists {
			orders[i].IsReadyForSplit = isReady
		}
	}

	// Логируем итоговые данные перед отправкой
	log.Printf("📤 Итоговые данные для отправки в веб-форму:")
	for _, order := range orders {
		for _, product := range order.Products {
			log.Printf("   Заказ %s: товар %d, marking_completed=%v, mandatory_marked=%v, gtd_required=%v",
				order.PostingNumber, product.ProductID, product.IsMarkingCompleted,
				product.IsMandatoryMarked, product.IsGtdRequired)
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
