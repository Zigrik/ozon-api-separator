package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"ozon-api-separator/internal/models"
)

// GetOrdersFilePath - возвращает путь к файлу состояния заказов
func GetOrdersFilePath() string {
	os.MkdirAll("orders", 0755)
	dateStr := time.Now().Format("2006_01_02")
	return filepath.Join("orders", fmt.Sprintf("orders_%s.json", dateStr))
}

// LoadCabinetState - загружает состояние кабинета из файла
func LoadCabinetState(cabinetKey string) (*models.CabinetState, error) {
	filePath := GetOrdersFilePath()

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
	filePath := GetOrdersFilePath()
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

// UpdateOrders - обновляет список заказов
func UpdateOrders(cabinetKey, cabinetName string, orders []models.Posting) error {
	state, err := LoadCabinetState(cabinetKey)
	if err != nil {
		return err
	}

	state.CabinetKey = cabinetKey
	state.CabinetName = cabinetName

	// Создаем карту существующих заказов в состоянии
	existingOrdersMap := make(map[string]*models.OrderState)
	for i := range state.Orders {
		existingOrdersMap[state.Orders[i].PostingNumber] = &state.Orders[i]
	}

	// Создаем карту новых заказов из API
	newOrdersMap := make(map[string]models.Posting)
	for _, order := range orders {
		newOrdersMap[order.PostingNumber] = order
	}

	updatedOrders := make([]models.OrderState, 0)

	// 1. Обновляем или добавляем заказы из API
	for _, order := range orders {
		var orderState models.OrderState

		if existing, exists := existingOrdersMap[order.PostingNumber]; exists {
			// Заказ уже есть - обновляем продукты
			orderState = *existing
			orderState.Products = convertProducts(order.Products)
			// Логируем только если это действительно новый заказ
		} else {
			// Новый заказ
			orderState = models.OrderState{
				PostingNumber: order.PostingNumber,
				IsDivided:     false,
				Products:      convertProducts(order.Products),
				Shipments:     make([]models.ShipmentState, 0),
				Errors:        make([]models.OrderError, 0),
			}
			log.Printf("➕ Новый заказ: %s", order.PostingNumber)
		}

		orderState.LabelsStatus = orderState.CalculateLabelsStatus()
		updatedOrders = append(updatedOrders, orderState)
	}

	// 2. Сохраняем заказы, которые уже НЕ в API
	for postingNumber, existing := range existingOrdersMap {
		if _, exists := newOrdersMap[postingNumber]; !exists {
			// Заказ уже не активен, сохраняем в истории
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
						Error:        nil,
					},
				})
			}

			state.Orders[i].LabelsStatus = needLabels
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

// convertProducts - конвертирует продукты из API в состояние
func convertProducts(products []models.Product) []models.ProductState {
	result := make([]models.ProductState, 0)
	for _, p := range products {
		result = append(result, models.ProductState{
			ProductID: p.ProductID,
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
