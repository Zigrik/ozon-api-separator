package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"ozon-api-separator/internal/config"
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
		log.Printf("[ERROR] Кабинет '%s' не настроен", cabinet.Name)
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	log.Printf("[INFO] Загрузка заказов для кабинета '%s'", cabinet.Name)

	orders, err := services.GetAwaitingPackagingOrders(cabinet)
	if err != nil {
		log.Printf("[ERROR] Ошибка загрузки заказов: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] Загружено %d заказов из Ozon API", len(orders))

	if err := services.UpdateOrders(cabinet.Key, cabinet.Name, orders); err != nil {
		log.Printf("[WARNING] Ошибка сохранения состояния: %v", err)
	}

	state, err := services.LoadCabinetState(cabinet.Key)
	if err != nil {
		log.Printf("[WARNING] Ошибка загрузки состояния: %v", err)
	}

	readyMap := make(map[string]bool)
	markingMap := make(map[string]map[int64]bool)

	if state != nil {
		for _, orderState := range state.Orders {
			readyMap[orderState.PostingNumber] = orderState.IsReadyForSplit
			for _, product := range orderState.Products {
				if markingMap[orderState.PostingNumber] == nil {
					markingMap[orderState.PostingNumber] = make(map[int64]bool)
				}
				markingMap[orderState.PostingNumber][product.ProductID] = product.Marking.IsCompleted
			}
		}
	}

	for i := range orders {
		order := &orders[i]

		needsCheck := false
		for _, p := range order.Products {
			if p.IsMandatoryMarked || p.IsGtdRequired {
				needsCheck = true
				break
			}
		}

		if !needsCheck {
			continue
		}

		markingStatus, err := services.CheckMarkingStatus(cabinet, order.PostingNumber)
		if err != nil {
			log.Printf("[WARNING] Ошибка проверки статуса маркировки для заказа %s: %v", order.PostingNumber, err)
			continue
		}

		if err := services.UpdateMarkingStatusFromAPI(cabinet.Key, order.PostingNumber, markingStatus); err != nil {
			log.Printf("[WARNING] Ошибка обновления статуса маркировки: %v", err)
		}

		for j := range order.Products {
			product := &order.Products[j]

			if product.ProductID == 0 && product.SKU != 0 {
				if isCompleted, exists := markingStatus[product.SKU]; exists && isCompleted {
					product.IsMarkingCompleted = true
					product.IsMandatoryMarked = false
					product.IsGtdRequired = false
					continue
				}
			}

			if isCompleted, exists := markingStatus[product.ProductID]; exists && isCompleted {
				product.IsMarkingCompleted = true
				product.IsMandatoryMarked = false
				product.IsGtdRequired = false
				continue
			}

			if !product.IsMarkingCompleted && product.SKU != 0 {
				for pid, isCompleted := range markingStatus {
					if pid == product.SKU && isCompleted {
						product.IsMarkingCompleted = true
						product.IsMandatoryMarked = false
						product.IsGtdRequired = false
						break
					}
				}
			}
		}
	}

	if markingMap != nil {
		for _, order := range orders {
			if markingMap[order.PostingNumber] != nil {
				for j := range order.Products {
					product := &order.Products[j]
					if isCompleted, exists := markingMap[order.PostingNumber][product.ProductID]; exists && isCompleted {
						product.IsMarkingCompleted = true
						product.IsMandatoryMarked = false
						product.IsGtdRequired = false
					}
					if !product.IsMarkingCompleted && product.SKU != 0 {
						for pid, isCompleted := range markingMap[order.PostingNumber] {
							if pid == product.SKU && isCompleted {
								product.IsMarkingCompleted = true
								product.IsMandatoryMarked = false
								product.IsGtdRequired = false
								break
							}
						}
					}
				}
			}
		}
	}

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

// shipOrders - общая функция для разделения заказов (HTTP-обработчик)
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
		log.Printf("[ERROR] Ошибка декодирования запроса: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cabinet := config.GetActiveConfig()

	ordersToShip := make([]struct {
		PostingNumber string
		Products      []struct {
			ProductID int64
			OfferID   string
			Quantity  int
		}
	}, len(req.Orders))

	for i, order := range req.Orders {
		ordersToShip[i].PostingNumber = order.PostingNumber
		for _, p := range order.Products {
			ordersToShip[i].Products = append(ordersToShip[i].Products, struct {
				ProductID int64
				OfferID   string
				Quantity  int
			}{
				ProductID: p.ProductID,
				OfferID:   p.OfferID,
				Quantity:  p.Quantity,
			})
		}
	}

	results, err := services.ShipOrdersInternal(cabinet, ordersToShip, wakeWorker)
	if err != nil {
		log.Printf("[ERROR] Ошибка разделения: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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

	if needLabels == 1 && wakeWorker {
		log.Printf("[INFO] Разделение и заказ этикеток завершены: успешно %d, ошибок %d", successCount, errorCount)
	} else {
		log.Printf("[INFO] Разделение завершено: успешно %d, ошибок %d", successCount, errorCount)
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
		log.Printf("[ERROR] Ошибка получения состояния заказа: %v", err)
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

	// Проверяем существование файла
	filePath := services.GetOrdersFilePath(cabinetKey)
	log.Printf("[DEBUG] Путь к файлу статистики: %s", filePath)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		log.Printf("[WARNING] Файл статистики не существует: %s", filePath)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":            "ok",
			"orders_divided":    0,
			"items_divided":     0,
			"labels_ordered":    0,
			"labels_downloaded": 0,
		})
		return
	}

	// Получаем статистику
	ordersDivided, itemsDivided, labelsOrdered, labelsDownloaded, err := services.GetTodayStats(cabinetKey)
	if err != nil {
		log.Printf("[ERROR] Ошибка получения статистики: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":            "ok",
			"orders_divided":    0,
			"items_divided":     0,
			"labels_ordered":    0,
			"labels_downloaded": 0,
		})
		return
	}

	log.Printf("[INFO] Статистика для кабинета %s: заказов=%d, товаров=%d, этикеток заказано=%d, скачано=%d",
		cabinetKey, ordersDivided, itemsDivided, labelsOrdered, labelsDownloaded)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "ok",
		"orders_divided":    ordersDivided,
		"items_divided":     itemsDivided,
		"labels_ordered":    labelsOrdered,
		"labels_downloaded": labelsDownloaded,
	})
}
