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
		log.Printf("📥 Попытка %d/%d: получение этикетки для заказа %s (task_id=%d)", attempt, maxRetries, postingNumber, taskID)

		content, err := getLabelByTaskID(cab, taskID)
		if err == nil {
			return content, nil
		}

		log.Printf("⚠️ Ошибка получения этикетки (попытка %d/%d): %v", attempt, maxRetries, err)

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

	if total == 0 {
		queue.Lock()
		queue.Status[jobID] = "completed"
		queue.Unlock()
		return
	}

	cab := config.GetActiveConfig()
	dataPath := config.GetDataPathForCabinet(cab.Key)

	// Ждём 10 секунд после разделения заказа
	log.Printf("⏳ Ожидание 10 секунд перед проверкой статуса...")
	time.Sleep(10 * time.Second)

	// Проверяем, что все заказы перешли в статус awaiting_deliver
	log.Printf("🔍 Проверка статуса %d заказов...", len(orders))
	readyOrders := make([]string, 0)

	for _, order := range orders {
		for attempt := 1; attempt <= 10; attempt++ {
			deliveredOrders, err := GetOrdersByStatus(cab, "awaiting_deliver")
			if err != nil {
				log.Printf("⚠️ Ошибка получения списка awaiting_deliver: %v", err)
				time.Sleep(3 * time.Second)
				continue
			}

			found := false
			for _, o := range deliveredOrders {
				if o.PostingNumber == order {
					readyOrders = append(readyOrders, order)
					log.Printf("✅ Заказ %s готов к скачиванию этикетки (статус awaiting_deliver)", order)
					found = true
					break
				}
			}

			if found {
				break
			}

			if attempt < 10 {
				log.Printf("⏳ Заказ %s ещё не в статусе awaiting_deliver, ожидание 3 секунды... (попытка %d/10)", order, attempt)
				time.Sleep(3 * time.Second)
			} else {
				log.Printf("⚠️ Заказ %s не перешёл в статус awaiting_deliver после 10 попыток", order)
			}
		}
	}

	if len(readyOrders) == 0 {
		log.Printf("❌ Ни один заказ не готов к скачиванию этикеток")
		queue.Lock()
		queue.Status[jobID] = "error"
		queue.Errors[jobID] = "Нет заказов в статусе awaiting_deliver"
		queue.Unlock()
		return
	}

	log.Printf("📦 Готово к скачиванию: %d из %d заказов", len(readyOrders), len(orders))

	// Получаем информацию о заказах для логирования названий товаров
	ordersInfo := make(map[string]*models.Posting)
	allOrders, err := GetAwaitingPackagingOrders(cab)
	if err == nil {
		for _, order := range allOrders {
			ordersInfo[order.PostingNumber] = &order
		}
	}

	deliveredOrders, err := GetOrdersByStatus(cab, "awaiting_deliver")
	if err == nil {
		for _, order := range deliveredOrders {
			if _, exists := ordersInfo[order.PostingNumber]; !exists {
				ordersInfo[order.PostingNumber] = &order
			}
		}
	}

	// ШАГ 1: Создаём все задачи (параллельно)
	log.Printf("📦 Шаг 1: создание %d задач на этикетки...", len(readyOrders))
	tasks := make(map[string]int64)
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5)

	for _, order := range readyOrders {
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
					time.Sleep(3 * time.Second)
				}
			}

			if err != nil {
				log.Printf("❌ Ошибка создания задачи для %s после 5 попыток: %v", o, err)
				return
			}

			mu.Lock()
			tasks[o] = taskID
			mu.Unlock()
			log.Printf("✅ Создана задача для %s: task_id=%d", o, taskID)
		}(order)
	}
	wg.Wait()
	log.Printf("📦 Шаг 1 завершён: создано %d задач", len(tasks))

	// ШАГ 2: Ждём 5 секунд после создания всех задач
	log.Printf("⏳ Ожидание 5 секунд перед скачиванием...")
	time.Sleep(5 * time.Second)

	// ШАГ 3: Скачиваем все этикетки
	log.Printf("📦 Шаг 2: скачивание %d этикеток...", len(tasks))
	completed := 0

	for _, order := range readyOrders {
		taskID, exists := tasks[order]
		if !exists {
			log.Printf("⚠️ Нет task_id для %s, пропускаем", order)
			completed++
			queue.Lock()
			queue.Progress[jobID] = completed
			queue.Unlock()
			continue
		}

		productName := ""
		if orderInfo, ok := ordersInfo[order]; ok && len(orderInfo.Products) > 0 {
			productName = orderInfo.Products[0].Name
		}

		content, err := GetLabelByTaskIDWithRetry(cab, taskID, order)
		if err != nil {
			log.Printf("❌ Ошибка получения этикетки для заказа %s (товар: %s): %v", order, productName, err)
			completed++
			queue.Lock()
			queue.Progress[jobID] = completed
			queue.Unlock()
			continue
		}

		parts := strings.Split(order, "-")
		folderName := strings.Join(parts[:len(parts)-1], "-")
		if folderName == "" {
			folderName = order
		}
		fileName := order + ".pdf"
		if err := SaveLabelToFile(dataPath, folderName, fileName, content); err != nil {
			log.Printf("❌ Ошибка сохранения этикетки для %s (товар: %s): %v", order, productName, err)
		} else {
			log.Printf("✅ Этикетка сохранена: %s/%s (товар: %s)", folderName, fileName, productName)
		}

		completed++
		queue.Lock()
		queue.Progress[jobID] = completed
		queue.Unlock()
	}

	queue.Lock()
	queue.Status[jobID] = "completed"
	queue.Unlock()
	log.Printf("✅ Задача %s завершена: %d/%d этикеток", jobID, completed, len(readyOrders))
}
