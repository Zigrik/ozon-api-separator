package services

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/models"
)

type CSVMarkItem struct {
	OrderNumber string
	OfferID     string
	Code        string
}

var processedOrders = struct {
	sync.RWMutex
	items map[string]bool
}{
	items: make(map[string]bool),
}

func StartAllAutoWorkers() {
	for key, cab := range config.AppConfig.Cabinets {
		if config.IsAutoModeEnabledForCabinet(key) {
			StartCSVWorkerForCabinet(cab)
		}
	}
}

func StartCSVWorkerForCabinet(cab *models.CabinetConfig) {
	config.CSVWorkersMutex.Lock()
	defer config.CSVWorkersMutex.Unlock()

	if config.CSVWorkerRunningMap[cab.Key] {
		return
	}

	stop := make(chan struct{})
	config.CSVWorkerStopChan[cab.Key] = stop
	config.CSVWorkerRunningMap[cab.Key] = true

	dataPath := cab.DataPath
	if dataPath == "" {
		dataPath = filepath.Join("data", cab.Key)
	}

	log.Printf("🤖 Авто-режим для кабинета %s запущен. Путь: %s", cab.Name, dataPath)

	go func(c *models.CabinetConfig, stopCh <-chan struct{}, path string) {
		ticker := time.NewTicker(config.MonitorInterval)
		defer ticker.Stop()

		cleanupTicker := time.NewTicker(1 * time.Hour)
		defer cleanupTicker.Stop()

		for {
			select {
			case <-ticker.C:
				if !config.IsAutoModeEnabledForCabinet(c.Key) {
					continue
				}
				processAutoMode(c, path)
			case <-cleanupTicker.C:
				processedOrders.Lock()
				processedOrders.items = make(map[string]bool)
				processedOrders.Unlock()
				log.Printf("🧹 Авто-режим: очищен кэш обработанных заказов")
			case <-stopCh:
				log.Printf("🛑 Авто-режим для кабинета %s остановлен", c.Name)
				return
			}
		}
	}(cab, stop, dataPath)
}

// StopCSVWorkerForCabinet - останавливает авто-режим для указанного кабинета
func StopCSVWorkerForCabinet(cabKey string) {
	config.CSVWorkersMutex.Lock()
	defer config.CSVWorkersMutex.Unlock()

	if ch, ok := config.CSVWorkerStopChan[cabKey]; ok {
		close(ch)
		delete(config.CSVWorkerStopChan, cabKey)
		delete(config.CSVWorkerRunningMap, cabKey)
		log.Printf("🛑 Авто-режим для кабинета %s остановлен (функция StopCSVWorkerForCabinet)", cabKey)
	}
}

// Получить префикс заказа (часть до последнего дефиса)
func getOrderPrefix(orderNumber string) string {
	parts := strings.Split(orderNumber, "-")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return orderNumber
}

// Проверить, существует ли папка для заказа (по префиксу)
func checkFolderExists(basePath, orderNumber string) bool {
	prefix := getOrderPrefix(orderNumber)
	folderPath := filepath.Join(basePath, prefix)
	info, err := os.Stat(folderPath)
	return err == nil && info.IsDir()
}

func processAutoMode(cab *models.CabinetConfig, basePath string) {
	// Получаем актуальный список заказов
	orders, err := GetAwaitingPackagingOrders(cab)
	if err != nil {
		log.Printf("⚠️ Авто-режим: ошибка получения списка заказов: %v", err)
		return
	}

	// Создаём карту существующих заказов по ПОЛНОМУ номеру и по ПРЕФИКСУ
	existingOrdersFull := make(map[string]bool)
	existingOrdersPrefix := make(map[string]bool)
	for _, order := range orders {
		existingOrdersFull[order.PostingNumber] = true
		prefix := getOrderPrefix(order.PostingNumber)
		existingOrdersPrefix[prefix] = true
	}

	// Сканируем CSV файлы в КОРНЕВОЙ папке кабинета
	files, err := os.ReadDir(basePath)
	if err != nil {
		return
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(file.Name()), ".csv") {
			continue
		}

		csvPath := filepath.Join(basePath, file.Name())
		processCSVFileInRoot(cab, basePath, csvPath, existingOrdersFull, existingOrdersPrefix)
	}
}

func processCSVFileInRoot(cab *models.CabinetConfig, basePath, csvPath string, existingOrdersFull, existingOrdersPrefix map[string]bool) {
	log.Printf("📄 Авто-режим: обработка CSV файла %s", csvPath)

	// Читаем все строки
	var allLines []string
	var validItems []CSVMarkItem
	var linesToRemove []int

	file, err := os.Open(csvPath)
	if err != nil {
		log.Printf("❌ Авто-режим: ошибка открытия CSV %s: %v", csvPath, err)
		return
	}

	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			allLines = append(allLines, line)
			continue
		}
		allLines = append(allLines, line)

		parts := strings.Fields(line) // разбиваем по пробелам
		if len(parts) < 3 {
			log.Printf("⚠️ Авто-режим: пропущена некорректная строка %d в CSV: %s", lineNum, line)
			continue
		}

		orderNumber := parts[0]
		offerID := parts[1]
		code := parts[2]

		// Проверяем, существует ли такой заказ (по полному номеру ИЛИ по префиксу)
		prefix := getOrderPrefix(orderNumber)
		orderExists := existingOrdersFull[orderNumber] || existingOrdersPrefix[prefix]

		if !orderExists {
			log.Printf("⚠️ Авто-режим: заказ %s не найден в актуальном списке, строка будет удалена", orderNumber)
			linesToRemove = append(linesToRemove, lineNum-1)
			continue
		}

		// Проверяем, существует ли папка для этого заказа
		if !checkFolderExists(basePath, orderNumber) {
			log.Printf("⚠️ Авто-режим: папка для заказа %s не существует, строка будет удалена", orderNumber)
			linesToRemove = append(linesToRemove, lineNum-1)
			continue
		}

		validItems = append(validItems, CSVMarkItem{
			OrderNumber: orderNumber,
			OfferID:     offerID,
			Code:        code,
		})
	}
	file.Close()

	// Удаляем невалидные строки
	if len(linesToRemove) > 0 {
		var newLines []string
		for i, line := range allLines {
			shouldRemove := false
			for _, removeIdx := range linesToRemove {
				if i == removeIdx {
					shouldRemove = true
					break
				}
			}
			if !shouldRemove && line != "" {
				newLines = append(newLines, line)
			}
		}

		if len(newLines) == 0 {
			os.Remove(csvPath)
			log.Printf("🗑️ Авто-режим: CSV файл %s удалён (все строки невалидны)", csvPath)
		} else {
			output := strings.Join(newLines, "\n")
			os.WriteFile(csvPath, []byte(output), 0644)
			log.Printf("💾 Авто-режим: CSV файл %s обновлён, удалено %d невалидных строк", csvPath, len(linesToRemove))
		}
	}

	if len(validItems) == 0 {
		return
	}

	// Группируем валидные строки по заказам
	ordersToProcess := make(map[string][]CSVMarkItem)
	for _, item := range validItems {
		prefix := getOrderPrefix(item.OrderNumber)
		ordersToProcess[prefix] = append(ordersToProcess[prefix], item)
	}

	// Обрабатываем каждый заказ
	for prefix, items := range ordersToProcess {
		// Находим полный номер заказа
		var fullOrderNumber string
		for fullNum := range existingOrdersFull {
			if getOrderPrefix(fullNum) == prefix {
				fullOrderNumber = fullNum
				break
			}
		}
		if fullOrderNumber == "" {
			log.Printf("⚠️ Авто-режим: не найден полный номер заказа для префикса %s", prefix)
			continue
		}

		// Проверяем, не обработан ли уже этот заказ
		processedOrders.RLock()
		alreadyProcessed := processedOrders.items[prefix]
		processedOrders.RUnlock()
		if alreadyProcessed {
			log.Printf("⏭️ Авто-режим: заказ %s уже обработан, пропускаем", fullOrderNumber)
			continue
		}

		processMarkingItemsForOrder(cab, fullOrderNumber, prefix, basePath, items)
	}
}

func processMarkingItemsForOrder(cab *models.CabinetConfig, fullOrderNumber, prefix, basePath string, items []CSVMarkItem) {
	log.Printf("📦 Авто-режим: обработка заказа %s, получено %d маркировок", fullOrderNumber, len(items))

	// Получаем актуальные данные заказа
	orders, err := GetAwaitingPackagingOrders(cab)
	if err != nil {
		log.Printf("❌ Авто-режим: ошибка получения списка заказов: %v", err)
		return
	}

	var targetOrder *models.Posting
	for i := range orders {
		if orders[i].PostingNumber == fullOrderNumber {
			targetOrder = &orders[i]
			break
		}
	}
	if targetOrder == nil {
		log.Printf("⚠️ Авто-режим: заказ %s не найден в актуальном списке", fullOrderNumber)
		return
	}

	// Добавляем маркировки
	var appliedMarks []CSVMarkItem
	for _, item := range items {
		for _, prod := range targetOrder.Products {
			if prod.OfferID == item.OfferID {
				productID := prod.ProductID
				if productID == 0 {
					productID = prod.SKU
				}
				if productID == 0 {
					log.Printf("⚠️ Авто-режим: не найден product_id для товара %s", item.OfferID)
					continue
				}
				if err := AddMarkingsForOrder(cab, fullOrderNumber, productID, 1, []string{item.Code}); err != nil {
					log.Printf("❌ Авто-режим: ошибка добавления марки %s для заказа %s: %v", item.Code, fullOrderNumber, err)
				} else {
					appliedMarks = append(appliedMarks, item)
					log.Printf("✅ Авто-режим: добавлена марка %s для товара %s в заказе %s", item.Code, prod.Name, fullOrderNumber)
				}
				break
			}
		}
	}

	if len(appliedMarks) == 0 {
		log.Printf("⚠️ Авто-режим: не удалось добавить ни одной марки для заказа %s", fullOrderNumber)
		return
	}

	// Удаляем использованные марки из CSV файла
	removeUsedMarksFromCSV(basePath, prefix, appliedMarks)

	// Проверяем, все ли требуемые маркировки добавлены
	allMarkingsCompleted := true
	for _, prod := range targetOrder.Products {
		if (prod.IsMandatoryMarked || prod.IsGtdRequired) && !prod.IsMarkingCompleted {
			allMarkingsCompleted = false
			break
		}
	}

	if !allMarkingsCompleted {
		log.Printf("⏳ Авто-режим: заказ %s ожидает дополнительные маркировки", fullOrderNumber)
		return
	}

	// Все маркировки добавлены - формируем упаковки и отправляем
	log.Printf("🚀 Авто-режим: все маркировки добавлены для заказа %s, формируем отправления", fullOrderNumber)

	packages := make([]ShipPackage, 0)
	for _, p := range targetOrder.Products {
		productID := p.ProductID
		if productID == 0 {
			productID = p.SKU
		}
		if productID == 0 {
			continue
		}
		for i := 0; i < p.Quantity; i++ {
			packages = append(packages, ShipPackage{
				Products: []ShipProduct{
					{
						ProductID: productID,
						Quantity:  1,
					},
				},
			})
		}
	}

	if len(packages) == 0 {
		log.Printf("⚠️ Авто-режим: нет товаров для отправки в заказе %s", fullOrderNumber)
		return
	}

	shipments, err := ShipOrder(cab, fullOrderNumber, packages)
	if err != nil {
		log.Printf("❌ Авто-режим: ошибка разделения заказа %s: %v", fullOrderNumber, err)
		return
	}

	log.Printf("✅ Авто-режим: заказ %s разделён на %d отправлений", fullOrderNumber, len(shipments))

	// Ждём 5 секунд перед заказом этикеток
	log.Printf("⏳ Ожидание 5 секунд перед заказом этикеток...")
	time.Sleep(5 * time.Second)

	// Запускаем ProcessLabelJob для ПОДЗАКАЗОВ
	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	queue := GetLabelQueue()
	queue.Lock()
	queue.Jobs[jobID] = shipments
	queue.Status[jobID] = "pending"
	queue.Total[jobID] = len(shipments)
	queue.Progress[jobID] = 0
	queue.StartTime[jobID] = time.Now()
	queue.Errors[jobID] = ""
	queue.FailedItems[jobID] = []string{}
	queue.Unlock()

	ProcessLabelJob(jobID, queue)

	processedOrders.Lock()
	processedOrders.items[prefix] = true
	processedOrders.Unlock()
}

func removeUsedMarksFromCSV(basePath, prefix string, usedMarks []CSVMarkItem) {
	// Создаем карту использованных марок
	usedMap := make(map[string]bool)
	for _, mark := range usedMarks {
		key := mark.OrderNumber + "|" + mark.OfferID + "|" + mark.Code
		usedMap[key] = true
	}

	// Ищем CSV файлы в папке заказа
	folderPath := filepath.Join(basePath, prefix)
	csvPath := findCSVFileInFolder(folderPath)
	if csvPath == "" {
		// Если нет CSV в папке заказа, ищем в корне кабинета
		csvPath = findCSVFileInRoot(basePath)
		if csvPath == "" {
			log.Printf("⚠️ Авто-режим: не найден CSV файл для удаления использованных марок")
			return
		}
	}

	// Читаем и фильтруем строки
	file, err := os.Open(csvPath)
	if err != nil {
		log.Printf("❌ Авто-режим: ошибка открытия CSV %s: %v", csvPath, err)
		return
	}

	var remainingLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			key := parts[0] + "|" + parts[1] + "|" + parts[2]
			if !usedMap[key] {
				remainingLines = append(remainingLines, line)
			}
		} else {
			remainingLines = append(remainingLines, line)
		}
	}
	file.Close()

	if len(remainingLines) == 0 {
		os.Remove(csvPath)
		log.Printf("🗑️ Авто-режим: CSV файл %s удалён (все строки обработаны)", csvPath)
	} else {
		output := strings.Join(remainingLines, "\n")
		os.WriteFile(csvPath, []byte(output), 0644)
		log.Printf("💾 Авто-режим: CSV файл %s обновлён, осталось %d строк", csvPath, len(remainingLines))
	}
}

func findCSVFileInFolder(folderPath string) string {
	files, err := os.ReadDir(folderPath)
	if err != nil {
		return ""
	}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".csv") {
			return filepath.Join(folderPath, file.Name())
		}
	}
	return ""
}

func findCSVFileInRoot(basePath string) string {
	files, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(strings.ToLower(file.Name()), ".csv") {
			return filepath.Join(basePath, file.Name())
		}
	}
	return ""
}
