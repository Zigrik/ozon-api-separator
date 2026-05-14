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
				return
			}
		}
	}(cab, stop, dataPath)
}

func processAutoMode(cab *models.CabinetConfig, basePath string) {
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		folderName := e.Name()
		folderPath := filepath.Join(basePath, folderName)

		processedOrders.RLock()
		alreadyProcessed := processedOrders.items[folderName]
		processedOrders.RUnlock()

		if alreadyProcessed {
			continue
		}

		requiresMarking := checkIfOrderRequiresMarking(cab, folderName)

		if !requiresMarking {
			log.Printf("🚀 Авто-режим: заказ %s не требует маркировки, делим сразу", folderName)
			processOrderWithoutMarking(cab, folderName, folderPath)
			processedOrders.Lock()
			processedOrders.items[folderName] = true
			processedOrders.Unlock()
			continue
		}

		processMarkingRequiredOrder(cab, folderName, folderPath)
	}
}

func checkIfOrderRequiresMarking(cab *models.CabinetConfig, orderPrefix string) bool {
	orders, err := GetAwaitingPackagingOrders(cab)
	if err != nil {
		log.Printf("⚠️ Ошибка получения списка заказов: %v", err)
		return false
	}

	prefix := orderPrefix + "-"
	for _, order := range orders {
		if strings.HasPrefix(order.PostingNumber, prefix) {
			for _, product := range order.Products {
				if product.IsMandatoryMarked || product.IsGtdRequired {
					return true
				}
			}
			return false
		}
	}
	return false
}

func processOrderWithoutMarking(cab *models.CabinetConfig, orderPrefix, folderPath string) {
	orders, err := GetAwaitingPackagingOrders(cab)
	if err != nil {
		log.Printf("❌ Ошибка получения списка заказов: %v", err)
		return
	}

	var targetOrder *models.Posting
	prefix := orderPrefix + "-"
	for i := range orders {
		if strings.HasPrefix(orders[i].PostingNumber, prefix) {
			targetOrder = &orders[i]
			break
		}
	}
	if targetOrder == nil {
		log.Printf("⚠️ Заказ с префиксом %s не найден в актуальном списке", orderPrefix)
		return
	}

	log.Printf("📦 Авто-режим: обработка заказа %s, товаров: %d", targetOrder.PostingNumber, len(targetOrder.Products))

	type shipItem struct {
		ProductID int64
		Quantity  int
		Name      string
	}
	var toShip []shipItem

	for _, p := range targetOrder.Products {
		productID := p.ProductID
		if productID == 0 {
			productID = p.SKU
			log.Printf("⚠️ Авто-режим: ProductID для товара '%s' был 0, используем SKU %d", p.Name, productID)
		}

		if productID == 0 {
			log.Printf("❌ Авто-режим: товар '%s' имеет ProductID=0 и SKU=0, пропускаем", p.Name)
			continue
		}

		toShip = append(toShip, shipItem{
			ProductID: productID,
			Quantity:  p.Quantity,
			Name:      p.Name,
		})

		log.Printf("📦 Авто-режим: товар '%s' (ProductID=%d, SKU=%d) кол-во=%d",
			p.Name, productID, p.SKU, p.Quantity)
	}

	if len(toShip) == 0 {
		log.Printf("⚠️ Авто-режим: нет товаров с корректным ProductID для заказа %s", targetOrder.PostingNumber)
		return
	}

	packages := make([]ShipPackage, 0)
	for _, p := range toShip {
		for i := 0; i < p.Quantity; i++ {
			packages = append(packages, ShipPackage{
				Products: []ShipProduct{
					{
						ProductID: p.ProductID,
						Quantity:  1,
					},
				},
			})
		}
	}

	log.Printf("📦 Авто-режим: создано %d упаковок для заказа %s", len(packages), targetOrder.PostingNumber)

	shipments, err := ShipOrder(cab, targetOrder.PostingNumber, packages)
	if err != nil {
		log.Printf("❌ Авто-режим: ошибка разделения заказа %s: %v", targetOrder.PostingNumber, err)

		productsLog := make([]string, len(targetOrder.Products))
		for i, p := range targetOrder.Products {
			productsLog[i] = fmt.Sprintf("%s (x%d)", p.Name, p.Quantity)
		}
		WriteToLog(orderPrefix, productsLog, nil, nil, false, false, err.Error())
		return
	}

	log.Printf("✅ Авто-режим: заказ %s разделён на %d отправлений", targetOrder.PostingNumber, len(shipments))

	// Ждём 10 секунд перед созданием задач на этикетки
	log.Printf("⏳ Ожидание 10 секунд перед созданием задач на этикетки...")
	time.Sleep(10 * time.Second)

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	queue := GetLabelQueue()
	queue.Lock()
	queue.Jobs[jobID] = shipments
	queue.Status[jobID] = "pending"
	queue.Total[jobID] = len(shipments)
	queue.Progress[jobID] = 0
	queue.StartTime[jobID] = time.Now()
	queue.Unlock()
	go ProcessLabelJob(jobID, queue)

	productsLog := make([]string, len(targetOrder.Products))
	for i, p := range targetOrder.Products {
		productsLog[i] = fmt.Sprintf("%s (x%d)", p.Name, p.Quantity)
	}
	WriteToLog(orderPrefix, productsLog, shipments, nil, false, true, "")

	processedOrders.Lock()
	processedOrders.items[orderPrefix] = true
	processedOrders.Unlock()
}

func processMarkingRequiredOrder(cab *models.CabinetConfig, orderPrefix, folderPath string) {
	files, err := os.ReadDir(folderPath)
	if err != nil {
		return
	}

	var csvFiles []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".csv") {
			csvFiles = append(csvFiles, f.Name())
		}
	}

	if len(csvFiles) == 0 {
		return
	}

	log.Printf("📄 Авто-режим: найдено %d CSV файлов в папке %s", len(csvFiles), orderPrefix)

	for _, csvFile := range csvFiles {
		csvPath := filepath.Join(folderPath, csvFile)
		processCSVFile(cab, orderPrefix, folderPath, csvPath)
	}
}

func processCSVFile(cab *models.CabinetConfig, orderPrefix, folderPath, csvPath string) {
	var items []CSVMarkItem
	var allLines []string

	file, err := os.Open(csvPath)
	if err != nil {
		log.Printf("❌ Авто-режим: ошибка открытия CSV %s: %v", csvPath, err)
		return
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		allLines = append(allLines, line)

		parts := strings.Fields(line)
		if len(parts) < 3 {
			log.Printf("⚠️ Авто-режим: пропущена некорректная строка в CSV: %s", line)
			continue
		}

		items = append(items, CSVMarkItem{
			OrderNumber: parts[0],
			OfferID:     parts[1],
			Code:        parts[2],
		})
	}
	file.Close()

	if len(items) == 0 {
		os.Remove(csvPath)
		log.Printf("🗑️ Авто-режим: удалён пустой CSV файл: %s", csvPath)
		return
	}

	var validItems []CSVMarkItem
	for _, item := range items {
		if item.OrderNumber == orderPrefix {
			validItems = append(validItems, item)
		} else {
			log.Printf("⚠️ Авто-режим: найден чужой заказ %s в CSV файле %s, строка будет удалена", item.OrderNumber, csvPath)
			WriteToLog(orderPrefix, nil, nil, []string{item.Code}, false, false, fmt.Sprintf("Чужой заказ %s в CSV", item.OrderNumber))
		}
	}

	if len(validItems) == 0 {
		if err := os.WriteFile(csvPath, []byte{}, 0644); err == nil {
			log.Printf("🗑️ Авто-режим: очищен CSV файл %s (не было строк для заказа %s)", csvPath, orderPrefix)
			os.Remove(csvPath)
		}
		return
	}

	orders, err := GetAwaitingPackagingOrders(cab)
	if err != nil {
		log.Printf("❌ Авто-режим: ошибка получения списка заказов: %v", err)
		return
	}

	var targetOrder *models.Posting
	prefix := orderPrefix + "-"
	for i := range orders {
		if strings.HasPrefix(orders[i].PostingNumber, prefix) {
			targetOrder = &orders[i]
			break
		}
	}
	if targetOrder == nil {
		log.Printf("⚠️ Авто-режим: заказ с префиксом %s не найден в актуальном списке", orderPrefix)
		saveFilteredCSV(csvPath, items, validItems)
		return
	}

	log.Printf("📦 Авто-режим: обработка заказа %s, товаров: %d", targetOrder.PostingNumber, len(targetOrder.Products))

	type appliedMark struct {
		OfferID string
		Code    string
	}
	var appliedMarks []appliedMark

	for _, item := range validItems {
		for _, prod := range targetOrder.Products {
			if prod.OfferID == item.OfferID {
				productID := prod.ProductID
				if productID == 0 {
					productID = prod.SKU
					log.Printf("⚠️ Авто-режим: ProductID для товара '%s' был 0, используем SKU %d", prod.Name, productID)
				}

				if productID == 0 {
					log.Printf("❌ Авто-режим: товар '%s' имеет ProductID=0 и SKU=0, пропускаем", prod.Name)
					continue
				}

				if err := AddMarkingsForOrder(cab, targetOrder.PostingNumber, productID, 1, []string{item.Code}); err != nil {
					log.Printf("❌ Авто-режим: ошибка добавления марки %s для заказа %s: %v", item.Code, targetOrder.PostingNumber, err)
				} else {
					appliedMarks = append(appliedMarks, appliedMark{OfferID: item.OfferID, Code: item.Code})
					log.Printf("✅ Авто-режим: добавлена марка %s для товара %s в заказе %s", item.Code, prod.Name, targetOrder.PostingNumber)
				}
				break
			}
		}
	}

	if len(appliedMarks) == 0 {
		log.Printf("⚠️ Авто-режим: не удалось добавить ни одной марки для заказа %s", targetOrder.PostingNumber)
		return
	}

	var toShip []struct {
		ProductID int64
		Quantity  int
	}
	for _, p := range targetOrder.Products {
		productID := p.ProductID
		if productID == 0 {
			productID = p.SKU
		}
		if productID == 0 {
			continue
		}
		toShip = append(toShip, struct {
			ProductID int64
			Quantity  int
		}{ProductID: productID, Quantity: p.Quantity})
	}

	if len(toShip) == 0 {
		log.Printf("⚠️ Авто-режим: нет товаров для отправки в заказе %s", targetOrder.PostingNumber)
		return
	}

	packages := make([]ShipPackage, 0)
	for _, p := range toShip {
		for i := 0; i < p.Quantity; i++ {
			packages = append(packages, ShipPackage{
				Products: []ShipProduct{
					{
						ProductID: p.ProductID,
						Quantity:  1,
					},
				},
			})
		}
	}

	log.Printf("📦 Авто-режим: создано %d упаковок для заказа %s", len(packages), targetOrder.PostingNumber)

	shipments, err := ShipOrder(cab, targetOrder.PostingNumber, packages)
	if err != nil {
		log.Printf("❌ Авто-режим: ошибка разделения заказа %s: %v", targetOrder.PostingNumber, err)

		productsLog := make([]string, len(targetOrder.Products))
		for i, p := range targetOrder.Products {
			productsLog[i] = fmt.Sprintf("%s (x%d)", p.OfferID, p.Quantity)
		}
		marksLog := make([]string, len(appliedMarks))
		for i, m := range appliedMarks {
			marksLog[i] = m.Code
		}
		WriteToLog(orderPrefix, productsLog, nil, marksLog, false, false, err.Error())
		return
	}

	log.Printf("✅ Авто-режим: заказ %s разделён на %d отправлений", targetOrder.PostingNumber, len(shipments))

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	queue := GetLabelQueue()
	queue.Lock()
	queue.Jobs[jobID] = shipments
	queue.Status[jobID] = "pending"
	queue.Total[jobID] = len(shipments)
	queue.Progress[jobID] = 0
	queue.StartTime[jobID] = time.Now()
	queue.Unlock()
	go ProcessLabelJob(jobID, queue)

	productsLog := make([]string, len(targetOrder.Products))
	for i, p := range targetOrder.Products {
		productsLog[i] = fmt.Sprintf("%s (x%d)", p.OfferID, p.Quantity)
	}
	marksLog := make([]string, len(appliedMarks))
	for i, m := range appliedMarks {
		marksLog[i] = m.Code
	}
	WriteToLog(orderPrefix, productsLog, shipments, marksLog, true, true, "")

	processedSet := make(map[string]bool)
	for _, item := range validItems {
		key := item.OrderNumber + "|" + item.OfferID + "|" + item.Code
		processedSet[key] = true
	}

	var remainingLines []string
	for _, line := range allLines {
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			key := parts[0] + "|" + parts[1] + "|" + parts[2]
			if !processedSet[key] {
				remainingLines = append(remainingLines, line)
			}
		}
	}

	if len(remainingLines) == 0 {
		os.Remove(csvPath)
		log.Printf("🗑️ Авто-режим: CSV файл %s удалён (все строки обработаны)", csvPath)
	} else {
		output := strings.Join(remainingLines, "\n")
		os.WriteFile(csvPath, []byte(output), 0644)
		log.Printf("💾 Авто-режим: CSV файл %s обновлён: осталось %d строк", csvPath, len(remainingLines))
	}

	processedOrders.Lock()
	processedOrders.items[orderPrefix] = true
	processedOrders.Unlock()
}

func saveFilteredCSV(csvPath string, allItems []CSVMarkItem, validItems []CSVMarkItem) {
	validSet := make(map[string]bool)
	for _, item := range validItems {
		key := item.OrderNumber + "|" + item.OfferID + "|" + item.Code
		validSet[key] = true
	}

	var lines []string
	for _, item := range allItems {
		key := item.OrderNumber + "|" + item.OfferID + "|" + item.Code
		if validSet[key] {
			lines = append(lines, fmt.Sprintf("%s %s %s", item.OrderNumber, item.OfferID, item.Code))
		}
	}

	if len(lines) == 0 {
		os.Remove(csvPath)
	} else {
		os.WriteFile(csvPath, []byte(strings.Join(lines, "\n")), 0644)
	}
}

func StopCSVWorkerForCabinet(cabKey string) {
	config.CSVWorkersMutex.Lock()
	defer config.CSVWorkersMutex.Unlock()
	if ch, ok := config.CSVWorkerStopChan[cabKey]; ok {
		close(ch)
		delete(config.CSVWorkerStopChan, cabKey)
		delete(config.CSVWorkerRunningMap, cabKey)
	}
}
