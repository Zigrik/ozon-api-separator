package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/models"
)

func CreateLabelTask(cab *models.CabinetConfig, postingNumber string) (int64, error) {
	body, err := MakeOzonRequest(cab, "POST",
		"https://api-seller.ozon.ru/v2/posting/fbs/package-label/create",
		models.CreateLabelRequest{PostingNumbers: []string{postingNumber}})
	if err != nil {
		return 0, err
	}
	var resp models.CreateLabelResponse
	json.Unmarshal(body, &resp)
	if len(resp.Result.Tasks) == 0 {
		return 0, fmt.Errorf("нет задач")
	}
	return resp.Result.Tasks[0].TaskID, nil
}

func getLabelByTaskID(cab *models.CabinetConfig, taskID int64) ([]byte, error) {
	body, err := MakeOzonRequest(cab, "POST",
		"https://api-seller.ozon.ru/v1/posting/fbs/package-label/get",
		models.GetLabelRequest{TaskID: taskID})
	if err != nil {
		return nil, err
	}
	var resp models.GetLabelResponse
	json.Unmarshal(body, &resp)
	if resp.Result.Status != "completed" || resp.Result.FileURL == "" {
		return nil, fmt.Errorf("статус %s", resp.Result.Status)
	}
	httpResp, err := http.Get(resp.Result.FileURL)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()
	return io.ReadAll(httpResp.Body)
}

func GetLabelByTaskIDWithRetry(cab *models.CabinetConfig, taskID int64, postingNumber string) ([]byte, error) {
	maxRetries := 5
	retryDelay := 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		content, err := getLabelByTaskID(cab, taskID)
		if err == nil {
			return content, nil
		}
		if attempt < maxRetries {
			time.Sleep(retryDelay)
		}
	}
	return nil, fmt.Errorf("не удалось получить этикетку для заказа %s после %d попыток", postingNumber, maxRetries)
}

func SaveLabelToFile(basePath, folderName, fileName string, content []byte) error {
	full := filepath.Join(basePath, folderName)
	if err := os.MkdirAll(full, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(full, fileName), content, 0644)
}

func ProcessLabelJob(jobID string, queue *models.LabelQueue) {
	config.LabelGenerationMutex.Lock()
	config.IsLabelGenerationRunning = true
	config.LabelGenerationMutex.Unlock()
	defer func() {
		config.LabelGenerationMutex.Lock()
		config.IsLabelGenerationRunning = false
		config.LabelGenerationMutex.Unlock()
	}()

	queue.Lock()
	orders := queue.Jobs[jobID]
	total := len(orders)
	queue.Status[jobID] = "processing"
	queue.Unlock()

	log.Printf("📦 ProcessLabelJob: начата обработка %d подзаказов", total)

	if total == 0 {
		queue.Lock()
		queue.Status[jobID] = "completed"
		queue.Unlock()
		return
	}

	cab := config.GetActiveConfig()
	dataPath := config.GetDataPathForCabinet(cab.Key)

	log.Printf("⏳ Ожидание 5 секунд перед созданием задач...")
	time.Sleep(5 * time.Second)

	// Создаём задачи
	log.Printf("📦 Создание %d задач на этикетки...", total)
	tasks := make(map[string]int64)
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5)
	var failedOrders []string

	for _, order := range orders {
		wg.Add(1)
		go func(o string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			var taskID int64
			var err error
			for attempt := 1; attempt <= 5; attempt++ {
				taskID, err = CreateLabelTask(cab, o)
				if err == nil {
					break
				}
				log.Printf("⚠️ Попытка %d/5 создания задачи для %s: %v", attempt, o, err)
				if attempt < 5 {
					time.Sleep(2 * time.Second)
				}
			}
			if err != nil {
				log.Printf("❌ Не удалось создать задачу для %s: %v", o, err)
				mu.Lock()
				failedOrders = append(failedOrders, o)
				mu.Unlock()
				return
			}
			mu.Lock()
			tasks[o] = taskID
			mu.Unlock()
			log.Printf("✅ Создана задача для %s: task_id=%d", o, taskID)
		}(order)
	}
	wg.Wait()

	// Удаляем failed заказы из списка orders
	var validOrders []string
	for _, order := range orders {
		isFailed := false
		for _, fo := range failedOrders {
			if fo == order {
				isFailed = true
				break
			}
		}
		if !isFailed {
			validOrders = append(validOrders, order)
		}
	}

	log.Printf("✅ Создано %d задач из %d, пропущено %d", len(tasks), total, len(failedOrders))

	if len(validOrders) == 0 {
		queue.Lock()
		queue.Status[jobID] = "error"
		queue.Errors[jobID] = fmt.Sprintf("Не удалось создать задачи для %d заказов", len(failedOrders))
		queue.Unlock()
		return
	}

	log.Printf("⏳ Ожидание 3 секунд перед скачиванием...")
	time.Sleep(3 * time.Second)

	// Скачиваем этикетки
	log.Printf("📥 Скачивание %d этикеток...", len(validOrders))
	completed := 0
	var downloadedOrders []string

	for _, order := range validOrders {
		taskID, exists := tasks[order]
		if !exists {
			continue
		}

		content, err := GetLabelByTaskIDWithRetry(cab, taskID, order)
		if err != nil {
			log.Printf("❌ Ошибка получения этикетки для %s: %v", order, err)
			continue
		}

		parts := strings.Split(order, "-")
		folderName := strings.Join(parts[:len(parts)-1], "-")
		if folderName == "" {
			folderName = order
		}
		fileName := order + ".pdf"

		if err := SaveLabelToFile(dataPath, folderName, fileName, content); err != nil {
			log.Printf("❌ Ошибка сохранения этикетки для %s: %v", order, err)
		} else {
			log.Printf("✅ Этикетка сохранена: %s/%s", folderName, fileName)
			downloadedOrders = append(downloadedOrders, order)
		}

		completed++
		queue.Lock()
		queue.Progress[jobID] = completed
		queue.Unlock()
		log.Printf("📊 Прогресс: %d/%d", completed, len(validOrders))
	}

	queue.Lock()
	if len(downloadedOrders) == len(validOrders) && len(failedOrders) == 0 {
		queue.Status[jobID] = "completed"
	} else if len(downloadedOrders) > 0 {
		queue.Status[jobID] = "partial"
		queue.Errors[jobID] = fmt.Sprintf("Скачано %d из %d этикеток, пропущено задач: %d", len(downloadedOrders), len(validOrders), len(failedOrders))
		queue.FailedItems[jobID] = failedOrders
	} else {
		queue.Status[jobID] = "error"
		queue.Errors[jobID] = fmt.Sprintf("Не скачано ни одной этикетки из %d", len(validOrders))
		queue.FailedItems[jobID] = failedOrders
	}
	queue.Unlock()

	log.Printf("🎉 ProcessLabelJob: завершено. Успешно: %d/%d, Ошибки: %d", len(downloadedOrders), len(validOrders), len(failedOrders))
}
