package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/models"
	"ozon-api-separator/internal/services"
)

var labelQueue = &models.LabelQueue{
	Jobs:        make(map[string][]string),
	Status:      make(map[string]string),
	Progress:    make(map[string]int),
	Total:       make(map[string]int),
	StartTime:   make(map[string]time.Time),
	Errors:      make(map[string]string),
	FailedItems: make(map[string][]string),
}

func HandleGetAvailableCodes(w http.ResponseWriter, r *http.Request) {
	config.CodesMutex.Lock()
	count := len(config.MarkingCodes)
	config.CodesMutex.Unlock()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"count":  count,
	})
}

func HandleStartLabelGenerationForShipments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config.LabelGenerationMutex.Lock()
	if config.IsLabelGenerationRunning {
		config.LabelGenerationMutex.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "busy",
			"message": "Уже выполняется генерация этикеток",
		})
		return
	}
	config.IsLabelGenerationRunning = true
	config.LabelGenerationMutex.Unlock()

	var req struct {
		PostingNumbers []string `json:"posting_numbers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.LabelGenerationMutex.Lock()
		config.IsLabelGenerationRunning = false
		config.LabelGenerationMutex.Unlock()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Убираем дубликаты
	uniqueNumbers := make(map[string]bool)
	for _, num := range req.PostingNumbers {
		uniqueNumbers[num] = true
	}

	uniqueList := make([]string, 0, len(uniqueNumbers))
	for num := range uniqueNumbers {
		uniqueList = append(uniqueList, num)
	}

	log.Printf("📦 Уникальных заказов для обработки: %d", len(uniqueList))

	cabinet := config.GetActiveConfig()

	// Разделяем заказы (каждый заказ только ОДИН раз)
	var allShipments []string

	for _, postingNumber := range uniqueList {
		// Получаем актуальный заказ в статусе awaiting_packaging
		orders, err := services.GetAwaitingPackagingOrders(cabinet)
		if err != nil {
			log.Printf("❌ Ошибка получения заказа %s: %v", postingNumber, err)
			continue
		}

		var targetOrder *models.Posting
		for i := range orders {
			if orders[i].PostingNumber == postingNumber {
				targetOrder = &orders[i]
				break
			}
		}

		if targetOrder == nil {
			log.Printf("⚠️ Заказ %s не найден в awaiting_packaging, пропускаем", postingNumber)
			continue
		}

		// Собираем товары
		packages := make([]services.ShipPackage, 0)
		for _, p := range targetOrder.Products {
			productID := p.ProductID
			if productID == 0 {
				productID = p.SKU
				log.Printf("⚠️ ProductID для товара '%s' был 0, используем SKU %d", p.Name, productID)
			}
			if productID == 0 {
				log.Printf("❌ Товар '%s' имеет ProductID=0 и SKU=0, пропускаем", p.Name)
				continue
			}
			for i := 0; i < p.Quantity; i++ {
				packages = append(packages, services.ShipPackage{
					Products: []services.ShipProduct{
						{
							ProductID: productID,
							Quantity:  1,
						},
					},
				})
			}
		}

		if len(packages) == 0 {
			log.Printf("⚠️ Нет товаров для заказа %s", postingNumber)
			continue
		}

		shipments, err := services.ShipOrder(cabinet, postingNumber, packages)
		if err != nil {
			log.Printf("❌ Ошибка разделения заказа %s: %v", postingNumber, err)
		} else {
			log.Printf("✅ Заказ %s разделён на %d отправлений", postingNumber, len(shipments))
			allShipments = append(allShipments, shipments...)
		}
	}

	if len(allShipments) == 0 {
		config.LabelGenerationMutex.Lock()
		config.IsLabelGenerationRunning = false
		config.LabelGenerationMutex.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": "Не удалось разделить ни одного заказа",
		})
		return
	}

	log.Printf("📦 Всего отправлений для этикеток: %d", len(allShipments))

	// Запускаем генерацию этикеток
	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	queue := services.GetLabelQueue()
	queue.Lock()
	queue.Jobs[jobID] = allShipments
	queue.Status[jobID] = "pending"
	queue.Total[jobID] = len(allShipments)
	queue.Progress[jobID] = 0
	queue.StartTime[jobID] = time.Now()
	queue.Errors[jobID] = ""
	queue.FailedItems[jobID] = []string{}
	queue.Unlock()

	go services.ProcessLabelJob(jobID, queue)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"job_id": jobID,
	})
}

func HandleGetLabelStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}
	labelQueue.Lock()
	defer labelQueue.Unlock()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       labelQueue.Status[jobID],
		"progress":     labelQueue.Progress[jobID],
		"total":        labelQueue.Total[jobID],
		"error":        labelQueue.Errors[jobID],
		"failed_items": labelQueue.FailedItems[jobID],
	})
}

func HandleCancelLabelGeneration(w http.ResponseWriter, r *http.Request) {
	config.LabelGenerationMutex.Lock()
	if config.IsLabelGenerationRunning {
		config.IsLabelGenerationRunning = false
		config.LabelGenerationMutex.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"message": "Генерация этикеток отменена",
		})
	} else {
		config.LabelGenerationMutex.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "idle",
			"message": "Нет активных задач",
		})
	}
}

func HandleRetryLabelGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config.LabelGenerationMutex.Lock()
	if config.IsLabelGenerationRunning {
		config.LabelGenerationMutex.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "busy",
			"message": "Уже выполняется генерация этикеток",
		})
		return
	}
	config.IsLabelGenerationRunning = true
	config.LabelGenerationMutex.Unlock()

	var req struct {
		JobID       string   `json:"job_id"`
		FailedItems []string `json:"failed_items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.LabelGenerationMutex.Lock()
		config.IsLabelGenerationRunning = false
		config.LabelGenerationMutex.Unlock()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	newJobID := fmt.Sprintf("retry_%d", time.Now().UnixNano())
	queue := services.GetLabelQueue()
	queue.Lock()
	queue.Jobs[newJobID] = req.FailedItems
	queue.Status[newJobID] = "pending"
	queue.Total[newJobID] = len(req.FailedItems)
	queue.Progress[newJobID] = 0
	queue.StartTime[newJobID] = time.Now()
	queue.Errors[newJobID] = ""
	queue.FailedItems[newJobID] = []string{}
	queue.Unlock()

	go services.ProcessLabelJob(newJobID, queue)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"job_id": newJobID,
	})
}
