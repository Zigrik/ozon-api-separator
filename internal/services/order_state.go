package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	os.MkdirAll("orders", 0755)
	dateStr := time.Now().Format("2006_01_02")
	return filepath.Join("orders", fmt.Sprintf("orders_%s_%s.json", cabinetKey, dateStr))
}

// LoadCabinetState - загружает состояние кабинета из файла
func LoadCabinetState(cabinetKey string) (*models.CabinetState, error) {
	stateMutex.Lock()
	defer stateMutex.Unlock()

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

// CheckFolderExists - проверяет существование папки для заказа
func CheckFolderExists(cabinetKey, postingNumber string) bool {
	cabinet := config.AppConfig.Cabinets[cabinetKey]
	if cabinet == nil {
		return false
	}

	dataPath := cabinet.DataPath
	if dataPath == "" {
		dataPath = filepath.Join("data", cabinetKey)
	}

	parts := strings.Split(postingNumber, "-")
	folderName := strings.Join(parts[:len(parts)-1], "-")
	if folderName == "" {
		folderName = postingNumber
	}

	folderPath := filepath.Join(dataPath, folderName)
	_, err := os.Stat(folderPath)
	return err == nil
}

// UpdateOrders - обновляет список заказов
func UpdateOrders(cabinetKey, cabinetName string, orders []models.Posting) error {
	state, err := LoadCabinetState(cabinetKey)
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

	for _, order := range orders {
		var orderState models.OrderState

		if existing, exists := existingOrdersMap[order.PostingNumber]; exists {
			orderState = *existing
			orderState.Products = convertProducts(order.Products)
			orderState.IsReadyForSplit = CheckFolderExists(cabinetKey, order.PostingNumber) || orderState.IsReadyForSplit
		} else {
			orderState = models.OrderState{
				PostingNumber:   order.PostingNumber,
				IsReadyForSplit: CheckFolderExists(cabinetKey, order.PostingNumber),
				IsDivided:       false,
				Products:        convertProducts(order.Products),
				Shipments:       make([]models.ShipmentState, 0),
				Errors:          make([]models.OrderError, 0),
			}
			log.Printf("➕ Новый заказ: %s (кабинет: %s)", order.PostingNumber, cabinetKey)
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
	return SaveCabinetState(state)
}

// UpdateOrderAfterShip - обновляет состояние заказа после разделения
func UpdateOrderAfterShip(cabinetKey, postingNumber string, shipments []string, productIDs []int64, needLabels int) error {
	state, err := LoadCabinetState(cabinetKey)
	if err != nil {
		return err
	}

	found := false
	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			state.Orders[i].IsReadyForSplit = true
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

			state.Orders[i].LabelsStatus = 1
			found = true
			log.Printf("✂️ Заказ %s разделен на %d отправлений", postingNumber, len(shipments))
			break
		}
	}

	if !found {
		return fmt.Errorf("заказ %s не найден в состоянии", postingNumber)
	}

	return SaveCabinetState(state)
}

// UpdateOrderLabel - обновляет состояние этикетки для подзаказа
func UpdateOrderLabel(cabinetKey, postingNumber string, taskID int64, isOrdered bool, filePath string) error {
	state, err := LoadCabinetState(cabinetKey)
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
				log.Printf("🏷️ Этикетка для %s: task_id=%d", postingNumber, taskID)
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

	return SaveCabinetState(state)
}

// AddOrderError - добавляет ошибку к заказу
func AddOrderError(cabinetKey, postingNumber, operation, errorMsg string) error {
	state, err := LoadCabinetState(cabinetKey)
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
			log.Printf("⚠️ Ошибка для заказа %s: %s", postingNumber, errorMsg)
			break
		}
	}

	return SaveCabinetState(state)
}

// GetOrderState - возвращает состояние конкретного заказа
func GetOrderState(cabinetKey, postingNumber string) (*models.OrderState, error) {
	state, err := LoadCabinetState(cabinetKey)
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
	state, err := LoadCabinetState(cabinetKey)
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
					log.Printf("🌍 Страна %s установлена для товара %d", countryCode, productID)
					break
				}
			}
			break
		}
	}

	return SaveCabinetState(state)
}

// UpdateGTDStatus - обновляет статус ГТД в состоянии
func UpdateGTDStatus(cabinetKey, postingNumber string, productID int64) error {
	state, err := LoadCabinetState(cabinetKey)
	if err != nil {
		return err
	}

	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			for j := range state.Orders[i].Products {
				if state.Orders[i].Products[j].ProductID == productID {
					state.Orders[i].Products[j].Marking.GtdAbsent = true
					log.Printf("📄 ГТД отмечено как отсутствующее для товара %d", productID)
					break
				}
			}
			break
		}
	}

	return SaveCabinetState(state)
}

// UpdateMarkingStatus - обновляет статус маркировки в состоянии
func UpdateMarkingStatus(cabinetKey, postingNumber string, productID int64, codes []string) error {
	state, err := LoadCabinetState(cabinetKey)
	if err != nil {
		return err
	}

	found := false
	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			for j := range state.Orders[i].Products {
				if state.Orders[i].Products[j].ProductID == productID {
					state.Orders[i].Products[j].Marking.IsCompleted = true
					state.Orders[i].Products[j].Marking.Codes = codes
					state.Orders[i].Products[j].Marking.Error = nil
					found = true
					log.Printf("🏷️ Маркировка обновлена в состоянии для товара %d в заказе %s (%d кодов)",
						productID, postingNumber, len(codes))
					break
				}
			}
			break
		}
	}

	if !found {
		log.Printf("⚠️ Товар %d не найден в заказе %s для обновления маркировки", productID, postingNumber)
	}

	return SaveCabinetState(state)
}

// UpdateMarkingStatusFromAPI - обновляет статус маркировки из API
func UpdateMarkingStatusFromAPI(cabinetKey, postingNumber string, markingStatus map[int64]bool) error {
	state, err := LoadCabinetState(cabinetKey)
	if err != nil {
		return err
	}

	for i := range state.Orders {
		if state.Orders[i].PostingNumber == postingNumber {
			for j := range state.Orders[i].Products {
				if isCompleted, exists := markingStatus[state.Orders[i].Products[j].ProductID]; exists {
					state.Orders[i].Products[j].Marking.IsCompleted = isCompleted
					if isCompleted {
						state.Orders[i].Products[j].Requirements.IsMandatoryMarked = false
						state.Orders[i].Products[j].Requirements.IsGtdRequired = false
						log.Printf("✅ Маркировка подтверждена для товара %d в заказе %s",
							state.Orders[i].Products[j].ProductID, postingNumber)
					}
				}
			}
			break
		}
	}

	return SaveCabinetState(state)
}

// processPendingLabels - обрабатывает заказы с labels_status = 1
func processPendingLabels() error {
	cabinet := config.GetActiveConfig()
	if cabinet == nil {
		return fmt.Errorf("активный кабинет не найден")
	}

	state, err := LoadCabinetState(cabinet.Key)
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
				log.Printf("❌ Ошибка заказа этикетки для %s (попытка %d): %v", shipment.PostingNumber, shipment.Label.RetryCount, err)
			} else {
				shipment.Label.TaskID = taskID
				shipment.Label.IsOrdered = true
				shipment.Label.RetryCount = 0
				shipment.Label.Error = nil
				log.Printf("✅ Этикетка для %s заказана, task_id=%d", shipment.PostingNumber, taskID)
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
				log.Printf("❌ Заказ %s: все подзаказы завершились ошибкой (labels_status=4)", order.PostingNumber)
			}
		}
	}

	if !found && atomic.LoadInt32(&KeyNeedLabels) == 0 {
		log.Println("ℹ️ Нет заказов с labels_status=1")
	}

	return SaveCabinetState(state)
}

// processPendingDownloads - обрабатывает заказы с labels_status = 2
func processPendingDownloads() error {
	cabinet := config.GetActiveConfig()
	if cabinet == nil {
		return fmt.Errorf("активный кабинет не найден")
	}

	state, err := LoadCabinetState(cabinet.Key)
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
				log.Printf("❌ Ошибка скачивания этикетки для %s (попытка %d): %v", shipment.PostingNumber, shipment.Label.RetryCount, err)
			} else {
				filePath, err := SaveLabelToFile(cabinet, shipment.PostingNumber, pdfData)
				if err != nil {
					shipment.Label.RetryCount++
					errMsg := err.Error()
					shipment.Label.Error = &errMsg
					hasErrors = true
					allDownloaded = false
					log.Printf("❌ Ошибка сохранения этикетки для %s (попытка %d): %v", shipment.PostingNumber, shipment.Label.RetryCount, err)
				} else {
					shipment.Label.FilePath = filePath
					shipment.Label.IsDownloaded = true
					shipment.Label.RetryCount = 0
					shipment.Label.Error = nil
					log.Printf("✅ Этикетка для %s скачана: %s", shipment.PostingNumber, filePath)
				}
			}
		}

		if allDownloaded {
			order.LabelsStatus = 3
			log.Printf("✅ Заказ %s: все этикетки скачаны", order.PostingNumber)
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
				log.Printf("❌ Заказ %s: все подзаказы завершились ошибкой скачивания (labels_status=4)", order.PostingNumber)
			}
		}
	}

	if !found && atomic.LoadInt32(&KeyDownloadLabels) == 0 {
		log.Println("ℹ️ Нет заказов с labels_status=2")
	}

	return SaveCabinetState(state)
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
