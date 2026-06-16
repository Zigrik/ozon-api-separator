package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ozon-api-separator/internal/models"
)

// MakeOzonRequest - выполняет HTTP-запрос к API Ozon
func MakeOzonRequest(cab *models.CabinetConfig, method, url string, body interface{}) ([]byte, error) {
	if cab.ClientID == "" || cab.APIKey == "" {
		return nil, fmt.Errorf("кабинет '%s' не настроен: отсутствуют ClientID или APIKey", cab.Name)
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации запроса: %w", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Client-Id", cab.ClientID)
	req.Header.Set("Api-Key", cab.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API вернул статус %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// GetAwaitingPackagingOrders - получает список заказов в статусе "ожидает упаковки"
func GetAwaitingPackagingOrders(cab *models.CabinetConfig) ([]models.Posting, error) {
	url := "https://api-seller.ozon.ru/v3/posting/fbs/unfulfilled/list"

	now := time.Now()
	cutoffFrom := now.AddDate(0, 0, -30)
	cutoffTo := now.AddDate(0, 0, 7)

	filter := models.PostingsFilter{
		Limit:  1000,
		Offset: 0,
	}
	filter.Filter.Status = "awaiting_packaging"
	filter.Filter.CutoffFrom = &cutoffFrom
	filter.Filter.CutoffTo = &cutoffTo

	respBody, err := MakeOzonRequest(cab, "POST", url, filter)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса к Ozon API: %w", err)
	}

	var response models.PostingsListResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа Ozon: %w", err)
	}

	// Обогащаем заказы информацией о требованиях
	for i := range response.Result.Postings {
		posting := &response.Result.Postings[i]

		if posting.Requirements != nil {
			markMap := make(map[int64]bool)
			gtdMap := make(map[int64]bool)
			cntMap := make(map[int64]bool)

			for _, id := range posting.Requirements.ProductsRequiringMandatoryMark {
				markMap[id] = true
			}
			for _, id := range posting.Requirements.ProductsRequiringGTD {
				gtdMap[id] = true
			}
			for _, id := range posting.Requirements.ProductsRequiringCountry {
				cntMap[id] = true
			}

			for j := range posting.Products {
				product := &posting.Products[j]
				pid := product.ProductID
				if pid == 0 {
					pid = product.SKU
				}
				product.IsMandatoryMarked = markMap[pid] || markMap[product.SKU]
				product.IsGtdRequired = gtdMap[pid] || gtdMap[product.SKU]
				product.IsCountryRequired = cntMap[pid] || cntMap[product.SKU]
			}
		}
	}

	return response.Result.Postings, nil
}

// ShipOrder - разделяет заказ на несколько отправлений
func ShipOrder(cab *models.CabinetConfig, postingNumber string, packages []models.ShipPackage) ([]string, error) {
	url := "https://api-seller.ozon.ru/v4/posting/fbs/ship"

	req := models.ShipRequest{
		PostingNumber: postingNumber,
		Packages:      packages,
	}

	respBody, err := MakeOzonRequest(cab, "POST", url, req)
	if err != nil {
		return nil, err
	}

	var response models.ShipResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	return response.Result, nil
}

// ============ ФУНКЦИИ ДЛЯ ЭТИКЕТОК ============

// CreateLabelTask - создает задачу на генерацию этикеток для заказов
// Возвращает ID задачи (task_id) для отслеживания статуса
func CreateLabelTask(cab *models.CabinetConfig, postingNumbers []string) (int64, error) {
	url := "https://api-seller.ozon.ru/v2/posting/fbs/package-label/create"

	req := models.CreateLabelRequest{
		PostingNumbers: postingNumbers,
	}

	respBody, err := MakeOzonRequest(cab, "POST", url, req)
	if err != nil {
		return 0, err
	}

	var response models.CreateLabelResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return 0, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	if len(response.Result.Tasks) == 0 {
		return 0, fmt.Errorf("не создано ни одной задачи для этикеток")
	}

	return response.Result.Tasks[0].TaskID, nil
}

// GetLabelByTaskID - получает этикетку по ID задачи
// Возвращает содержимое PDF-файла и ошибку, если этикетка не готова
func GetLabelByTaskID(cab *models.CabinetConfig, taskID int64) ([]byte, error) {
	url := "https://api-seller.ozon.ru/v1/posting/fbs/package-label/get"

	req := models.GetLabelRequest{
		TaskID: taskID,
	}

	respBody, err := MakeOzonRequest(cab, "POST", url, req)
	if err != nil {
		return nil, err
	}

	var response models.GetLabelResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	// Проверяем статус этикетки
	if response.Result.Status != "completed" {
		return nil, fmt.Errorf("этикетка ещё не готова, статус: %s", response.Result.Status)
	}

	if response.Result.FileURL == "" {
		return nil, fmt.Errorf("URL для скачивания этикетки пуст")
	}

	// Скачиваем PDF по URL
	pdfResp, err := http.Get(response.Result.FileURL)
	if err != nil {
		return nil, fmt.Errorf("ошибка скачивания PDF: %w", err)
	}
	defer pdfResp.Body.Close()

	if pdfResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PDF вернул статус %d", pdfResp.StatusCode)
	}

	pdfData, err := io.ReadAll(pdfResp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения PDF: %w", err)
	}

	return pdfData, nil
}

// GetLabelByTaskIDWithRetry - получает этикетку с повторными попытками
// Делает до 5 попыток с интервалом 2 секунды
func GetLabelByTaskIDWithRetry(cab *models.CabinetConfig, taskID int64, maxRetries int, retryDelay time.Duration) ([]byte, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		pdfData, err := GetLabelByTaskID(cab, taskID)
		if err == nil {
			return pdfData, nil
		}

		// Если этикетка не готова - пробуем снова
		if attempt < maxRetries {
			time.Sleep(retryDelay)
			continue
		}
		return nil, fmt.Errorf("не удалось получить этикетку после %d попыток: %w", maxRetries, err)
	}
	return nil, fmt.Errorf("не удалось получить этикетку")
}

// SaveLabelToFile - сохраняет PDF этикетки в файл
// Путь: dataPath/postingNumber/postingNumber.pdf
func SaveLabelToFile(cab *models.CabinetConfig, postingNumber string, pdfData []byte) (string, error) {
	// Определяем путь для сохранения
	dataPath := cab.DataPath
	if dataPath == "" {
		dataPath = filepath.Join("data", cab.Key)
	}

	// Создаем папку для заказа
	folderPath := filepath.Join(dataPath, postingNumber)
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		return "", fmt.Errorf("ошибка создания папки: %w", err)
	}

	// Сохраняем файл
	filePath := filepath.Join(folderPath, postingNumber+".pdf")
	if err := os.WriteFile(filePath, pdfData, 0644); err != nil {
		return "", fmt.Errorf("ошибка сохранения файла: %w", err)
	}

	return filePath, nil
}

// GetLabelStatus - проверяет статус задачи по ID
func GetLabelStatus(cab *models.CabinetConfig, taskID int64) (string, error) {
	url := "https://api-seller.ozon.ru/v1/posting/fbs/package-label/get"

	req := models.GetLabelRequest{
		TaskID: taskID,
	}

	respBody, err := MakeOzonRequest(cab, "POST", url, req)
	if err != nil {
		return "", err
	}

	var response models.GetLabelResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return "", fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	return response.Result.Status, nil
}
