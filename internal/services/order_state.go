package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/models"
)

var stateMutex sync.Mutex

// GetOrdersFilePath - возвращает путь к файлу состояния для конкретного кабинета
func GetOrdersFilePath(cabinetKey string) string {
	ordersPath := config.GetOrdersPath()
	os.MkdirAll(ordersPath, 0755)
	dateStr := time.Now().Format("2006_01_02")
	return filepath.Join(ordersPath, fmt.Sprintf("orders_%s_%s.json", cabinetKey, dateStr))
}

// LoadCabinetState - загружает состояние кабинета из файла
func LoadCabinetState(cabinetKey string) (*models.CabinetState, error) {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	return loadCabinetStateUnlocked(cabinetKey)
}

// loadCabinetStateUnlocked - загружает состояние без блокировки
func loadCabinetStateUnlocked(cabinetKey string) (*models.CabinetState, error) {
	filePath := GetOrdersFilePath(cabinetKey)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return &models.CabinetState{
			CabinetKey:  cabinetKey,
			CabinetName: "",
			LastUpdated: time.Now(),
			Orders:      make([]models.OrderState, 0),
		}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения файла: %w", err)
	}

	var state models.CabinetState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	return &state, nil
}

// SaveCabinetState - сохраняет состояние кабинета в файл
func SaveCabinetState(state *models.CabinetState) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	return saveCabinetStateUnlocked(state)
}

// saveCabinetStateUnlocked - сохраняет состояние без блокировки
func saveCabinetStateUnlocked(state *models.CabinetState) error {
	filePath := GetOrdersFilePath(state.CabinetKey)
	state.LastUpdated = time.Now()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("ошибка сериализации: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("ошибка сохранения файла: %w", err)
	}

	return nil
}

// GetTodayStats - возвращает статистику из текущего файла состояния
func GetTodayStats(cabinetKey string) (ordersDivided, itemsDivided, labelsOrdered, labelsDownloaded int, err error) {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		log.Printf("[ERROR] Ошибка загрузки состояния для %s: %v", cabinetKey, err)
		return 0, 0, 0, 0, err
	}

	ordersDivided = 0
	itemsDivided = 0
	labelsOrdered = 0
	labelsDownloaded = 0

	for _, order := range state.Orders {
		if !order.IsDivided {
			continue
		}

		ordersDivided++

		for _, product := range order.Products {
			itemsDivided += product.Quantity
		}

		for _, shipment := range order.Shipments {
			if shipment.Label.IsOrdered {
				labelsOrdered++
			}
			if shipment.Label.IsDownloaded {
				labelsDownloaded++
			}
		}
	}

	log.Printf("[INFO] Статистика для %s: заказов=%d, товаров=%d, этикеток заказано=%d, скачано=%d",
		cabinetKey, ordersDivided, itemsDivided, labelsOrdered, labelsDownloaded)

	return ordersDivided, itemsDivided, labelsOrdered, labelsDownloaded, nil
}

// CheckFolderExists - проверяет существование папки для заказа в labels
func CheckFolderExists(cabinetKey, postingNumber string) bool {
	labelsPath := config.GetLabelsPathForCabinet(cabinetKey)

	parts := strings.Split(postingNumber, "-")
	folderName := strings.Join(parts[:len(parts)-1], "-")
	if folderName == "" {
		folderName = postingNumber
	}

	folderPath := filepath.Join(labelsPath, folderName)
	_, err := os.Stat(folderPath)
	return err == nil
}

// getOrderPrefix - возвращает префикс заказа (без хвостика -N)
func getOrderPrefix(orderNumber string) string {
	parts := strings.Split(orderNumber, "-")
	if len(parts) < 3 {
		return orderNumber
	}
	lastPart := parts[len(parts)-1]
	if _, err := strconv.Atoi(lastPart); err == nil {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return orderNumber
}

// truncateString - обрезает строку для логирования
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// UpdateOrders - обновляет список заказов
func UpdateOrders(cabinetKey, cabinetName string, orders []models.Posting) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		return err
	}

	state.CabinetKey = cabinetKey
	state.CabinetName = cabinetName

	existingOrdersMap := make(map[string]*models.OrderState)
	for i := range state.Orders {
		existingOrdersMap[state.Orders[i].PostingNumber] = &state.Orders[i]
	}

	newOrdersMap := make(map[string]models.Posting)
	for _, order := range orders {
		newOrdersMap[order.PostingNumber] = order
	}

	updatedOrders := make([]models.OrderState, 0)

	// Получаем ID складов из .env
	warehouseUN := config.GetWarehouseUN()
	warehouseRev := config.GetWarehouseREV()

	for _, order := range orders {
		var orderState models.OrderState

		// Определяем отображаемое название склада
		warehouseDisplayName := ""
		if order.WarehouseID == warehouseUN {
			warehouseDisplayName = "Ун."
		} else if order.WarehouseID == warehouseRev {
			warehouseDisplayName = "Рев."
		}

		if existing, exists := existingOrdersMap[order.PostingNumber]; exists {
			orderState = *existing
			orderState.Products = convertProducts(order.Products)
			orderState.IsReadyForSplit = CheckFolderExists(cabinetKey, order.PostingNumber) || orderState.IsReadyForSplit
			// Обновляем данные склада
			orderState.WarehouseID = order.WarehouseID
			orderState.WarehouseName = warehouseDisplayName
			orderState.IntegrationType = order.IntegrationType
		} else {
			orderState = models.OrderState{
				PostingNumber:   order.PostingNumber,
				WarehouseID:     order.WarehouseID,
				WarehouseName:   warehouseDisplayName,
				IntegrationType: order.IntegrationType,
				IsReadyForSplit: CheckFolderExists(cabinetKey, order.PostingNumber),
				IsDivided:       false,
				Products:        convertProducts(order.Products),
				Shipments:       make([]models.ShipmentState, 0),
				Errors:          make([]models.OrderError, 0),
			}
			log.Printf("[INFO] Новый заказ: %s (кабинет: %s, склад: %s, ID: %d)",
				order.PostingNumber, cabinetKey, warehouseDisplayName, order.WarehouseID)
		}

		orderState.LabelsStatus = orderState.CalculateLabelsStatus()
		updatedOrders = append(updatedOrders, orderState)
	}

	for postingNumber, existing := range existingOrdersMap {
		if _, exists := newOrdersMap[postingNumber]; !exists {
			updatedOrders = append(updatedOrders, *existing)
		}
	}

	state.Orders = updatedOrders
	return saveCabinetStateUnlocked(state)
}

// UpdateOrderAfterShip - обновляет состояние заказа после разделения
func UpdateOrderAfterShip(cabinetKey, postingNumber string, shipments []string, productIDs []int64, needLabels int) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	log.Printf("[DEBUG] UpdateOrderAfterShip: cabinet=%s, posting=%s, shipments=%d", cabinetKey, postingNumber, len(shipments))

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		log.Printf("[ERROR] UpdateOrderAfterShip: ошибка загрузки состояния: %v", err)
		return err
	}

	found := false
	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			log.Printf("[DEBUG] UpdateOrderAfterShip: найден заказ %s в состоянии, is_divided=%v", postingNumber, state.Orders[i].IsDivided)

			state.Orders[i].IsReadyForSplit = false
			state.Orders[i].IsDivided = true

			for _, shipment := range shipments {
				state.Orders[i].Shipments = append(state.Orders[i].Shipments, models.ShipmentState{
					PostingNumber: shipment,
					ProductIDs:    productIDs,
					Label: models.LabelState{
						TaskID:       0,
						IsOrdered:    false,
						IsDownloaded: false,
						FilePath:     "",
						RetryCount:   0,
						Error:        nil,
					},
				})
			}

			state.Orders[i].LabelsStatus = needLabels
			found = true
			log.Printf("[INFO] Заказ %s разделен на %d отправлений, is_divided=true, labels_status=%d",
				postingNumber, len(shipments), needLabels)
			break
		}
	}

	if !found {
		log.Printf("[ERROR] UpdateOrderAfterShip: заказ %s НЕ НАЙДЕН в состоянии", postingNumber)
		return fmt.Errorf("заказ %s не найден в состоянии", postingNumber)
	}

	if err := saveCabinetStateUnlocked(state); err != nil {
		log.Printf("[ERROR] UpdateOrderAfterShip: ошибка сохранения состояния: %v", err)
		return err
	}

	log.Printf("[DEBUG] UpdateOrderAfterShip: состояние успешно сохранено для заказа %s", postingNumber)
	return nil
}

// UpdateOrderLabel - обновляет состояние этикетки для подзаказа
func UpdateOrderLabel(cabinetKey, postingNumber string, taskID int64, isOrdered bool, filePath string) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		return err
	}

	found := false
	for i := range state.Orders {
		for j := range state.Orders[i].Shipments {
			if state.Orders[i].Shipments[j].PostingNumber == postingNumber {
				state.Orders[i].Shipments[j].Label.TaskID = taskID
				state.Orders[i].Shipments[j].Label.IsOrdered = isOrdered
				if filePath != "" {
					state.Orders[i].Shipments[j].Label.FilePath = filePath
					state.Orders[i].Shipments[j].Label.IsDownloaded = true
				}
				state.Orders[i].LabelsStatus = state.Orders[i].CalculateLabelsStatus()
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		return fmt.Errorf("подзаказ %s не найден в состоянии", postingNumber)
	}

	return saveCabinetStateUnlocked(state)
}

// AddOrderError - добавляет ошибку к заказу
func AddOrderError(cabinetKey, postingNumber, operation, errorMsg string) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		return err
	}

	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			state.Orders[i].Errors = append(state.Orders[i].Errors, models.OrderError{
				PostingNumber: postingNumber,
				Operation:     operation,
				Error:         errorMsg,
				Timestamp:     time.Now().Format(time.RFC3339),
			})
			log.Printf("[ERROR] Ошибка для заказа %s: %s", postingNumber, errorMsg)
			break
		}
	}

	return saveCabinetStateUnlocked(state)
}

// GetOrderState - возвращает состояние конкретного заказа
func GetOrderState(cabinetKey, postingNumber string) (*models.OrderState, error) {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		return nil, err
	}

	for _, order := range state.Orders {
		if order.PostingNumber == postingNumber {
			return &order, nil
		}
	}

	return nil, fmt.Errorf("заказ %s не найден", postingNumber)
}

// UpdateCountryStatus - обновляет статус страны в состоянии
func UpdateCountryStatus(cabinetKey, postingNumber string, productID int64, countryCode string) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		return err
	}

	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			for j := range state.Orders[i].Products {
				if state.Orders[i].Products[j].ProductID == productID {
					state.Orders[i].Products[j].Country.IsCompleted = true
					code := countryCode
					state.Orders[i].Products[j].Country.Code = &code
					state.Orders[i].Products[j].Country.Error = nil
					break
				}
			}
			break
		}
	}

	return saveCabinetStateUnlocked(state)
}

// UpdateGTDStatus - обновляет статус ГТД в состоянии
func UpdateGTDStatus(cabinetKey, postingNumber string, productID int64) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		return err
	}

	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			for j := range state.Orders[i].Products {
				if state.Orders[i].Products[j].ProductID == productID {
					state.Orders[i].Products[j].Marking.GtdAbsent = true
					break
				}
			}
			break
		}
	}

	return saveCabinetStateUnlocked(state)
}

// UpdateMarkingStatus - обновляет статус маркировки в состоянии
func UpdateMarkingStatus(cabinetKey, postingNumber string, productID int64, codes []string) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		return err
	}

	found := false
	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			for j := range state.Orders[i].Products {
				product := &state.Orders[i].Products[j]
				if product.ProductID == productID || product.SKU == productID {
					product.Marking.IsCompleted = true
					product.Marking.Codes = codes
					product.Marking.Error = nil
					product.Requirements.IsMandatoryMarked = false
					product.Requirements.IsGtdRequired = false
					found = true
					log.Printf("[INFO] Маркировка добавлена для товара %d в заказе %s", productID, postingNumber)
					break
				}
			}
			break
		}
	}

	if !found {
		log.Printf("[WARNING] Товар %d не найден в заказе %s для обновления маркировки", productID, postingNumber)
	}

	return saveCabinetStateUnlocked(state)
}

// UpdateMarkingStatusFromAPI - обновляет статус маркировки из API
func UpdateMarkingStatusFromAPI(cabinetKey, postingNumber string, markingStatus map[int64]bool) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinetKey)
	if err != nil {
		return err
	}

	updated := false
	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			for j := range state.Orders[i].Products {
				product := &state.Orders[i].Products[j]

				if product.ProductID == 0 && product.SKU != 0 {
					if isCompleted, exists := markingStatus[product.SKU]; exists && isCompleted {
						product.Marking.IsCompleted = true
						product.Requirements.IsMandatoryMarked = false
						product.Requirements.IsGtdRequired = false
						updated = true
						continue
					}
				}

				if isCompleted, exists := markingStatus[product.ProductID]; exists && isCompleted {
					product.Marking.IsCompleted = true
					product.Requirements.IsMandatoryMarked = false
					product.Requirements.IsGtdRequired = false
					updated = true
					continue
				}

				if !product.Marking.IsCompleted && product.SKU != 0 {
					for pid, isCompleted := range markingStatus {
						if pid == product.SKU && isCompleted {
							product.Marking.IsCompleted = true
							product.Requirements.IsMandatoryMarked = false
							product.Requirements.IsGtdRequired = false
							updated = true
							break
						}
					}
				}
			}
			break
		}
	}

	if updated {
		log.Printf("[INFO] Статус маркировки обновлен для заказа %s", postingNumber)
	}

	return saveCabinetStateUnlocked(state)
}

// processPendingLabels - обрабатывает заказы с labels_status = 1
func processPendingLabels() error {
	// Небольшая задержка перед началом обработки
	time.Sleep(2 * time.Second)

	cabinet := config.GetActiveConfig()
	if cabinet == nil {
		return fmt.Errorf("активный кабинет не найден")
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinet.Key)
	if err != nil {
		return err
	}

	found := false
	for i := range state.Orders {
		if state.Orders[i].LabelsStatus != 1 {
			continue
		}

		if !state.Orders[i].IsDivided || len(state.Orders[i].Shipments) == 0 {
			continue
		}

		found = true
		order := &state.Orders[i]
		allOrdered := true
		hasErrors := false
		maxRetries := 5

		for j := range order.Shipments {
			shipment := &order.Shipments[j]

			if shipment.Label.IsOrdered {
				continue
			}

			if shipment.Label.RetryCount >= maxRetries {
				hasErrors = true
				allOrdered = false
				errMsg := fmt.Sprintf("превышено число попыток (%d)", maxRetries)
				shipment.Label.Error = &errMsg
				continue
			}

			taskID, err := CreateLabelTask(cabinet, []string{shipment.PostingNumber})
			if err != nil {
				shipment.Label.RetryCount++
				errMsg := err.Error()
				shipment.Label.Error = &errMsg
				hasErrors = true
				allOrdered = false
				log.Printf("[ERROR] Ошибка заказа этикетки для %s (попытка %d): %v", shipment.PostingNumber, shipment.Label.RetryCount, err)
			} else {
				shipment.Label.TaskID = taskID
				shipment.Label.IsOrdered = true
				shipment.Label.RetryCount = 0
				shipment.Label.Error = nil
				log.Printf("[INFO] Этикетка для %s заказана, task_id=%d", shipment.PostingNumber, taskID)
			}
		}

		if allOrdered {
			order.LabelsStatus = 2
			WakeDownloadWorker()
		} else if hasErrors {
			allFailed := true
			for _, s := range order.Shipments {
				if !s.Label.IsOrdered && s.Label.RetryCount < maxRetries {
					allFailed = false
					break
				}
			}
			if allFailed {
				order.LabelsStatus = 4
				log.Printf("[ERROR] Заказ %s: все подзаказы завершились ошибкой", order.PostingNumber)
			}
		}
	}

	if !found && atomic.LoadInt32(&KeyNeedLabels) == 0 {
		log.Println("[INFO] Нет заказов с labels_status=1")
	}

	return saveCabinetStateUnlocked(state)
}

// processPendingDownloads - обрабатывает заказы с labels_status = 2
func processPendingDownloads() error {
	// Небольшая задержка перед началом обработки
	time.Sleep(2 * time.Second)

	cabinet := config.GetActiveConfig()
	if cabinet == nil {
		return fmt.Errorf("активный кабинет не найден")
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	state, err := loadCabinetStateUnlocked(cabinet.Key)
	if err != nil {
		return err
	}

	found := false
	for i := range state.Orders {
		if state.Orders[i].LabelsStatus != 2 {
			continue
		}

		if !state.Orders[i].IsDivided || len(state.Orders[i].Shipments) == 0 {
			continue
		}

		found = true
		order := &state.Orders[i]
		allDownloaded := true
		hasErrors := false
		maxRetries := 5

		for j := range order.Shipments {
			shipment := &order.Shipments[j]

			if shipment.Label.IsDownloaded {
				continue
			}

			if !shipment.Label.IsOrdered {
				allDownloaded = false
				continue
			}

			if shipment.Label.RetryCount >= maxRetries {
				hasErrors = true
				allDownloaded = false
				errMsg := fmt.Sprintf("превышено число попыток скачивания (%d)", maxRetries)
				shipment.Label.Error = &errMsg
				continue
			}

			if shipment.Label.TaskID == 0 {
				allDownloaded = false
				errMsg := "нет task_id для скачивания"
				shipment.Label.Error = &errMsg
				continue
			}

			pdfData, err := GetLabelByTaskIDWithRetry(cabinet, shipment.Label.TaskID, 3, 2*time.Second)
			if err != nil {
				shipment.Label.RetryCount++
				errMsg := err.Error()
				shipment.Label.Error = &errMsg
				hasErrors = true
				allDownloaded = false
				log.Printf("[ERROR] Ошибка скачивания этикетки для %s (попытка %d): %v", shipment.PostingNumber, shipment.Label.RetryCount, err)
			} else {
				filePath, err := SaveLabelToFile(cabinet, shipment.PostingNumber, pdfData)
				if err != nil {
					shipment.Label.RetryCount++
					errMsg := err.Error()
					shipment.Label.Error = &errMsg
					hasErrors = true
					allDownloaded = false
					log.Printf("[ERROR] Ошибка сохранения этикетки для %s (попытка %d): %v", shipment.PostingNumber, shipment.Label.RetryCount, err)
				} else {
					shipment.Label.FilePath = filePath
					shipment.Label.IsDownloaded = true
					shipment.Label.RetryCount = 0
					shipment.Label.Error = nil
					log.Printf("[INFO] Этикетка для %s скачана", shipment.PostingNumber)
				}
			}
		}

		if allDownloaded {
			order.LabelsStatus = 3
			log.Printf("[INFO] Заказ %s: все этикетки скачаны", order.PostingNumber)
		} else if hasErrors {
			allFailed := true
			for _, s := range order.Shipments {
				if !s.Label.IsDownloaded && s.Label.RetryCount < maxRetries {
					allFailed = false
					break
				}
			}
			if allFailed {
				order.LabelsStatus = 4
				log.Printf("[ERROR] Заказ %s: все подзаказы завершились ошибкой скачивания", order.PostingNumber)
			}
		}
	}

	if !found && atomic.LoadInt32(&KeyDownloadLabels) == 0 {
		log.Println("[INFO] Нет заказов с labels_status=2")
	}

	return saveCabinetStateUnlocked(state)
}

// processReadyForSplitOrders - обрабатывает заказы с is_ready_for_split = true для ВСЕХ кабинетов
func processReadyForSplitOrders() error {
	var totalProcessed int

	for key, cabinet := range config.AppConfig.Cabinets {
		if cabinet.ClientID == "" || cabinet.APIKey == "" {
			continue
		}

		if !config.IsAutoModeEnabledForCabinet(key) {
			continue
		}

		// Получаем заказы для кабинета (без фильтра по складам)
		orders, err := GetAwaitingPackagingOrders(cabinet, []int64{})
		if err != nil {
			log.Printf("[WARNING] Авто-разделение: ошибка загрузки заказов для кабинета %s: %v", key, err)
			continue
		}

		// Обновляем состояние (сохраняем заказы в JSON)
		if err := UpdateOrders(key, cabinet.Name, orders); err != nil {
			log.Printf("[WARNING] Авто-разделение: ошибка сохранения для кабинета %s: %v", key, err)
			continue
		}

		log.Printf("[INFO] Авто-разделение [%s]: загружено %d заказов", key, len(orders))

		// Загружаем состояние из файла
		state, err := LoadCabinetState(key)
		if err != nil {
			log.Printf("[WARNING] Авто-разделение: ошибка загрузки состояния для кабинета %s: %v", key, err)
			continue
		}

		processed := 0
		for i := range state.Orders {
			order := &state.Orders[i]

			if !order.IsReadyForSplit || order.IsDivided || len(order.Products) == 0 {
				continue
			}

			log.Printf("[INFO] Авто-разделение [%s]: обработка заказа %s (склад: %s)", key, order.PostingNumber, order.WarehouseName)

			needsMarking := false
			for _, product := range order.Products {
				if product.Requirements.IsMandatoryMarked || product.Requirements.IsGtdRequired {
					needsMarking = true
					break
				}
			}

			if !needsMarking {
				if err := autoSplitOrder(key, order); err != nil {
					log.Printf("[ERROR] Авто-разделение [%s]: ошибка разделения заказа %s: %v", key, order.PostingNumber, err)
					continue
				}
				processed++
				continue
			}

			if err := processMarkingForOrder(key, order); err != nil {
				log.Printf("[WARNING] Авто-разделение [%s]: ошибка обработки маркировки для заказа %s: %v", key, order.PostingNumber, err)
				continue
			}

			// Перезагружаем состояние
			freshState, _ := LoadCabinetState(key)
			var freshOrder *models.OrderState
			for j := range freshState.Orders {
				if freshState.Orders[j].PostingNumber == order.PostingNumber {
					freshOrder = &freshState.Orders[j]
					break
				}
			}

			if freshOrder == nil {
				continue
			}

			allMarkingCompleted := true
			for _, product := range freshOrder.Products {
				if (product.Requirements.IsMandatoryMarked || product.Requirements.IsGtdRequired) && !product.Marking.IsCompleted {
					allMarkingCompleted = false
					break
				}
			}

			if allMarkingCompleted {
				if err := autoSplitOrder(key, freshOrder); err != nil {
					log.Printf("[ERROR] Авто-разделение [%s]: ошибка разделения заказа %s: %v", key, freshOrder.PostingNumber, err)
					continue
				}
				processed++
			} else {
				log.Printf("[INFO] Авто-разделение [%s]: заказ %s ожидает маркировки", key, order.PostingNumber)
			}
		}

		if processed > 0 {
			log.Printf("[INFO] Авто-разделение [%s]: обработано %d заказов", key, processed)
			totalProcessed += processed
		}
	}

	if totalProcessed > 0 {
		return nil
	}
	return nil
}

// processMarkingForOrder - обрабатывает маркировку для заказа из .txt файлов
func processMarkingForOrder(cabinetKey string, order *models.OrderState) error {
	labelsPath := config.GetLabelsPathForCabinet(cabinetKey)
	folderName := order.GetFolderName()
	folderPath := filepath.Join(labelsPath, folderName)

	var txtFiles []string

	if files, err := os.ReadDir(folderPath); err == nil {
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".txt") {
				txtFiles = append(txtFiles, filepath.Join(folderPath, f.Name()))
			}
		}
	}

	if len(txtFiles) == 0 {
		if files, err := os.ReadDir(labelsPath); err == nil {
			for _, f := range files {
				if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".txt") {
					txtFiles = append(txtFiles, filepath.Join(labelsPath, f.Name()))
				}
			}
		}
	}

	if len(txtFiles) == 0 {
		return nil
	}

	log.Printf("[INFO] Авто-разделение [%s]: найден .txt файл для заказа %s", cabinetKey, order.PostingNumber)

	type MarkItem struct {
		OrderNumber string
		OfferID     string
		Code        string
	}
	var allItems []MarkItem
	var allLines []string
	var filePaths []string

	for _, txtPath := range txtFiles {
		data, err := os.ReadFile(txtPath)
		if err != nil {
			continue
		}
		filePaths = append(filePaths, txtPath)

		content := string(data)
		lines := strings.Split(content, "\n")

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			allLines = append(allLines, line)

			idx1 := strings.Index(line, "  ")
			if idx1 == -1 {
				continue
			}

			orderNum := strings.TrimSpace(line[:idx1])
			remaining := line[idx1+2:]

			idx2 := strings.LastIndex(remaining, "  ")
			if idx2 == -1 {
				continue
			}

			offerID := strings.TrimSpace(remaining[:idx2])
			code := strings.TrimSpace(remaining[idx2+2:])

			if !strings.Contains(orderNum, "-") {
				continue
			}

			if len(code) < 30 {
				continue
			}

			allItems = append(allItems, MarkItem{
				OrderNumber: orderNum,
				OfferID:     offerID,
				Code:        code,
			})
		}
	}

	if len(allItems) == 0 {
		return nil
	}

	prefix := order.GetFolderName()

	var validItems []MarkItem
	var linesToRemove []int

	for idx, item := range allItems {
		itemPrefix := getOrderPrefix(item.OrderNumber)
		if itemPrefix == prefix {
			validItems = append(validItems, item)
			linesToRemove = append(linesToRemove, idx)
		}
	}

	if len(validItems) == 0 {
		return nil
	}

	log.Printf("[INFO] Авто-разделение [%s]: найдено %d маркировок для заказа %s", cabinetKey, len(validItems), order.PostingNumber)

	marksByOffer := make(map[string][]string)
	for _, item := range validItems {
		marksByOffer[item.OfferID] = append(marksByOffer[item.OfferID], item.Code)
	}

	cabinet := config.AppConfig.Cabinets[cabinetKey]
	if cabinet == nil {
		return fmt.Errorf("кабинет %s не найден", cabinetKey)
	}

	allMarkingAdded := true

	for offerID, codes := range marksByOffer {
		for j := range order.Products {
			product := &order.Products[j]
			if product.OfferID == offerID && (product.Requirements.IsMandatoryMarked || product.Requirements.IsGtdRequired) {
				if len(codes) >= product.Quantity {
					productID := product.ProductID
					if productID == 0 {
						productID = product.SKU
					}
					if productID == 0 {
						log.Printf("[ERROR] Авто-разделение [%s]: не удалось определить product_id для товара %s", cabinetKey, offerID)
						allMarkingAdded = false
						break
					}

					if err := AddMarkingsForOrder(cabinet, order.PostingNumber, productID, product.Quantity, codes[:product.Quantity]); err != nil {
						log.Printf("[ERROR] Авто-разделение [%s]: ошибка добавления маркировки для товара %s: %v", cabinetKey, offerID, err)
						allMarkingAdded = false
					} else {
						product.Marking.IsCompleted = true
						product.Requirements.IsMandatoryMarked = false
						product.Requirements.IsGtdRequired = false
						log.Printf("[INFO] Авто-разделение [%s]: добавлена маркировка для товара %s", cabinetKey, offerID)
					}
				} else {
					log.Printf("[WARNING] Авто-разделение [%s]: недостаточно марок для товара %s: нужно %d, есть %d", cabinetKey, offerID, product.Quantity, len(codes))
					allMarkingAdded = false
				}
				break
			}
		}
	}

	if !allMarkingAdded {
		return fmt.Errorf("не все маркировки добавлены для заказа %s", order.PostingNumber)
	}

	stateMutex.Lock()
	defer stateMutex.Unlock()

	fullState, err := loadCabinetStateUnlocked(cabinetKey)
	if err == nil {
		for i := range fullState.Orders {
			if fullState.Orders[i].PostingNumber == order.PostingNumber {
				fullState.Orders[i].Products = order.Products
				break
			}
		}
		saveCabinetStateUnlocked(fullState)
	}

	if len(linesToRemove) > 0 && len(filePaths) > 0 {
		var newLines []string
		for i, line := range allLines {
			shouldRemove := false
			for _, idx := range linesToRemove {
				if i == idx {
					shouldRemove = true
					break
				}
			}
			if !shouldRemove {
				newLines = append(newLines, line)
			}
		}

		if len(newLines) == 0 {
			os.Remove(filePaths[0])
			log.Printf("[INFO] Авто-разделение [%s]: .txt файл удалён (все строки обработаны)", cabinetKey)
		} else {
			os.WriteFile(filePaths[0], []byte(strings.Join(newLines, "\n")), 0644)
		}
	}

	return nil
}

// autoSplitOrder - автоматически разделяет заказ для указанного кабинета
func autoSplitOrder(cabinetKey string, order *models.OrderState) error {
	if order.IsDivided {
		return nil
	}

	cabinet := config.AppConfig.Cabinets[cabinetKey]
	if cabinet == nil {
		return fmt.Errorf("кабинет %s не найден", cabinetKey)
	}

	var products []struct {
		ProductID int64
		OfferID   string
		Quantity  int
	}

	for _, product := range order.Products {
		if product.IsReadyForShipment() {
			productID := product.ProductID
			if productID == 0 {
				productID = product.SKU
			}
			if productID == 0 {
				continue
			}
			products = append(products, struct {
				ProductID int64
				OfferID   string
				Quantity  int
			}{
				ProductID: productID,
				OfferID:   product.OfferID,
				Quantity:  product.Quantity,
			})
		}
	}

	if len(products) == 0 {
		return fmt.Errorf("нет товаров для отправки")
	}

	ordersToShip := []struct {
		PostingNumber string
		Products      []struct {
			ProductID int64
			OfferID   string
			Quantity  int
		}
	}{
		{
			PostingNumber: order.PostingNumber,
			Products:      products,
		},
	}

	results, err := ShipOrdersInternal(cabinet, ordersToShip, true)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		return fmt.Errorf("не получен результат разделения")
	}

	if results[0]["status"] == "error" {
		return fmt.Errorf("ошибка разделения: %s", results[0]["error"])
	}

	log.Printf("[INFO] Авто-разделение [%s]: заказ %s разделён успешно", cabinetKey, order.PostingNumber)
	return nil
}

// convertProducts - конвертирует продукты из API в состояние
func convertProducts(products []models.Product) []models.ProductState {
	result := make([]models.ProductState, 0)
	for _, p := range products {
		result = append(result, models.ProductState{
			ProductID: p.ProductID,
			SKU:       p.SKU,
			OfferID:   p.OfferID,
			Quantity:  p.Quantity,
			Price:     p.GetPriceFloat(), // ← сохраняем цену
			Requirements: models.ProductRequirement{
				IsMandatoryMarked: p.IsMandatoryMarked,
				IsGtdRequired:     p.IsGtdRequired,
				IsCountryRequired: p.IsCountryRequired,
			},
			Marking: models.MarkingState{
				IsCompleted: p.IsMarkingCompleted,
				Codes:       make([]string, 0),
				GtdAbsent:   false,
				Error:       nil,
			},
			Country: models.CountryState{
				IsCompleted: false,
				Code:        nil,
				Error:       nil,
			},
		})
	}
	return result
}
