package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ozon-api-separator/internal/config"
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

// CreateLabelTask - создает задачу на генерацию этикеток
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

// GetLabelByTaskID - получает этикетку по ID задачи
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

	if response.Result.Status != "completed" {
		return nil, fmt.Errorf("этикетка ещё не готова, статус: %s", response.Result.Status)
	}

	if response.Result.FileURL == "" {
		return nil, fmt.Errorf("URL для скачивания этикетки пуст")
	}

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
func GetLabelByTaskIDWithRetry(cab *models.CabinetConfig, taskID int64, maxRetries int, retryDelay time.Duration) ([]byte, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		pdfData, err := GetLabelByTaskID(cab, taskID)
		if err == nil {
			return pdfData, nil
		}

		if attempt < maxRetries {
			time.Sleep(retryDelay)
			continue
		}
		return nil, fmt.Errorf("не удалось получить этикетку после %d попыток: %w", maxRetries, err)
	}
	return nil, fmt.Errorf("не удалось получить этикетку")
}

// SaveLabelToFile - сохраняет PDF этикетки в файл
func SaveLabelToFile(cab *models.CabinetConfig, postingNumber string, pdfData []byte) (string, error) {
	// Используем LabelsPath из конфига кабинета
	labelsPath := cab.LabelsPath
	if labelsPath == "" {
		labelsPath = config.GetLabelsPathForCabinet(cab.Key)
	}

	parts := strings.Split(postingNumber, "-")
	folderName := strings.Join(parts[:len(parts)-1], "-")
	if folderName == "" {
		folderName = postingNumber
	}

	folderPath := filepath.Join(labelsPath, folderName)
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		return "", fmt.Errorf("ошибка создания папки: %w", err)
	}

	filePath := filepath.Join(folderPath, postingNumber+".pdf")
	if err := os.WriteFile(filePath, pdfData, 0644); err != nil {
		return "", fmt.Errorf("ошибка сохранения файла: %w", err)
	}

	return filePath, nil
}

// ============ ФУНКЦИИ ДЛЯ СТРАНЫ ============

// GetCountriesList - получает список стран производителей
func GetCountriesList(cab *models.CabinetConfig) ([]models.CountryInfo, error) {
	url := "https://api-seller.ozon.ru/v2/posting/fbs/product/country/list"
	respBody, err := MakeOzonRequest(cab, "POST", url, map[string]interface{}{})
	if err != nil {
		return getDefaultCountries(), nil
	}

	var response struct {
		Result []struct {
			Name           string `json:"name"`
			CountryISOCode string `json:"country_iso_code"`
		} `json:"result"`
	}

	if err := json.Unmarshal(respBody, &response); err != nil {
		return getDefaultCountries(), nil
	}

	countries := make([]models.CountryInfo, 0)
	for _, c := range response.Result {
		if c.Name != "" && c.CountryISOCode != "" {
			countries = append(countries, models.CountryInfo{
				Name: c.Name,
				Code: c.CountryISOCode,
			})
		}
	}

	if len(countries) == 0 {
		return getDefaultCountries(), nil
	}
	return countries, nil
}

func getDefaultCountries() []models.CountryInfo {
	return []models.CountryInfo{
		{Name: "Россия", Code: "RU"},
		{Name: "Китай", Code: "CN"},
		{Name: "Германия", Code: "DE"},
		{Name: "Япония", Code: "JP"},
		{Name: "США", Code: "US"},
		{Name: "Италия", Code: "IT"},
		{Name: "Франция", Code: "FR"},
		{Name: "Польша", Code: "PL"},
		{Name: "Турция", Code: "TR"},
		{Name: "Вьетнам", Code: "VN"},
	}
}

// SetCountry - устанавливает страну производителя
func SetCountry(cab *models.CabinetConfig, postingNumber string, productID int64, countryCode string) error {
	url := "https://api-seller.ozon.ru/v2/posting/fbs/product/country/set"
	countryCode = strings.TrimSpace(strings.ToUpper(countryCode))

	req := models.SetCountryRequest{
		PostingNumber:  postingNumber,
		ProductID:      productID,
		CountryISOCode: countryCode,
	}

	_, err := MakeOzonRequest(cab, "POST", url, req)
	if err != nil {
		return err
	}

	if err := UpdateCountryStatus(cab.Key, postingNumber, productID, countryCode); err != nil {
		log.Printf("[WARNING] Ошибка обновления состояния страны: %v", err)
	}

	return nil
}

// ============ ФУНКЦИИ ДЛЯ ГТД ============

// GetExemplarIDs - получает exemplar_id для товаров в заказе
func GetExemplarIDs(cab *models.CabinetConfig, postingNumber string) (*models.ExemplarCreateResponse, error) {
	body, err := MakeOzonRequest(cab, "POST",
		"https://api-seller.ozon.ru/v6/fbs/posting/product/exemplar/create-or-get",
		models.ExemplarCreateRequest{PostingNumber: postingNumber})
	if err != nil {
		return nil, err
	}

	var resp models.ExemplarCreateResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	return &resp, nil
}

// SetGTDAsAbsent - отмечает ГТД как отсутствующее
func SetGTDAsAbsent(cab *models.CabinetConfig, postingNumber string, productID int64) error {
	exemplars, err := GetExemplarIDs(cab, postingNumber)
	if err != nil {
		return fmt.Errorf("ошибка получения exemplar_id: %w", err)
	}

	var exemplarIDs []int64
	for _, p := range exemplars.Products {
		if p.ProductID == productID {
			for _, e := range p.Exemplars {
				exemplarIDs = append(exemplarIDs, e.ExemplarID)
			}
			break
		}
	}

	if len(exemplarIDs) == 0 {
		return fmt.Errorf("не найдены exemplar_id для товара %d в заказе %s", productID, postingNumber)
	}

	type GTDExemplar struct {
		ExemplarID   int64 `json:"exemplar_id"`
		IsGTDAbsent  bool  `json:"is_gtd_absent"`
		IsRNPTAbsent bool  `json:"is_rnpt_absent"`
	}

	type GTDProductExemplar struct {
		ProductID int64         `json:"product_id"`
		Exemplars []GTDExemplar `json:"exemplars"`
	}

	request := struct {
		PostingNumber string               `json:"posting_number"`
		Products      []GTDProductExemplar `json:"products"`
	}{
		PostingNumber: postingNumber,
		Products: []GTDProductExemplar{
			{
				ProductID: productID,
				Exemplars: make([]GTDExemplar, 0),
			},
		},
	}

	for _, id := range exemplarIDs {
		request.Products[0].Exemplars = append(request.Products[0].Exemplars, GTDExemplar{
			ExemplarID:   id,
			IsGTDAbsent:  true,
			IsRNPTAbsent: true,
		})
	}

	_, err = MakeOzonRequest(cab, "POST", "https://api-seller.ozon.ru/v6/fbs/posting/product/exemplar/set", request)
	if err != nil {
		return err
	}

	if err := UpdateGTDStatus(cab.Key, postingNumber, productID); err != nil {
		log.Printf("[WARNING] Ошибка обновления состояния ГТД: %v", err)
	}

	return nil
}

// ============ ФУНКЦИИ ДЛЯ МАРКИРОВКИ ============

// CheckMarkingStatus - проверяет статус маркировки для заказа
func CheckMarkingStatus(cab *models.CabinetConfig, postingNumber string) (map[int64]bool, error) {
	url := "https://api-seller.ozon.ru/v5/fbs/posting/product/exemplar/status"

	req := models.ExemplarStatusRequest{
		PostingNumber: postingNumber,
	}

	respBody, err := MakeOzonRequest(cab, "POST", url, req)
	if err != nil {
		return nil, err
	}

	var response models.ExemplarStatusResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	result := make(map[int64]bool)
	for _, product := range response.Products {
		hasValidMarks := false

		for _, exemplar := range product.Exemplars {
			if len(exemplar.Marks) > 0 {
				allValid := true
				for _, mark := range exemplar.Marks {
					if mark.CheckStatus != "passed" && mark.CheckStatus != "valid" {
						allValid = false
						break
					}
					if len(mark.ErrorCodes) > 0 {
						allValid = false
						break
					}
				}
				if allValid {
					hasValidMarks = true
				}
			}
		}

		result[product.ProductID] = hasValidMarks
	}

	return result, nil
}

// AddMarkingsForOrder - добавляет маркировку для товара
func AddMarkingsForOrder(cab *models.CabinetConfig, postingNumber string, productID int64, quantity int, codes []string) error {
	exemplars, err := GetExemplarIDs(cab, postingNumber)
	if err != nil {
		return fmt.Errorf("ошибка получения exemplar_id: %w", err)
	}

	var ids []int64
	for _, p := range exemplars.Products {
		if p.ProductID == productID {
			for _, e := range p.Exemplars {
				ids = append(ids, e.ExemplarID)
			}
			break
		}
	}

	if len(ids) == 0 {
		return fmt.Errorf("не найдены exemplar_id для товара %d в заказе %s", productID, postingNumber)
	}

	actualQuantity := len(ids)
	if actualQuantity < quantity {
		if len(codes) > actualQuantity {
			codes = codes[:actualQuantity]
		}
		quantity = actualQuantity
	}

	marks := make([]models.Mark, quantity)
	for i := 0; i < quantity; i++ {
		marks[i] = models.Mark{
			Mark:     codes[i],
			MarkType: "mandatory_mark",
		}
	}

	request := models.MarkingSetRequest{
		PostingNumber: postingNumber,
		Products: []struct {
			ProductID int64 `json:"product_id"`
			Exemplars []struct {
				ExemplarID   int64         `json:"exemplar_id"`
				IsGTDAbsent  bool          `json:"is_gtd_absent"`
				IsRNPTAbsent bool          `json:"is_rnpt_absent"`
				Marks        []models.Mark `json:"marks"`
			} `json:"exemplars"`
		}{
			{
				ProductID: productID,
				Exemplars: make([]struct {
					ExemplarID   int64         `json:"exemplar_id"`
					IsGTDAbsent  bool          `json:"is_gtd_absent"`
					IsRNPTAbsent bool          `json:"is_rnpt_absent"`
					Marks        []models.Mark `json:"marks"`
				}, 0),
			},
		},
	}

	for i, id := range ids[:quantity] {
		request.Products[0].Exemplars = append(request.Products[0].Exemplars, struct {
			ExemplarID   int64         `json:"exemplar_id"`
			IsGTDAbsent  bool          `json:"is_gtd_absent"`
			IsRNPTAbsent bool          `json:"is_rnpt_absent"`
			Marks        []models.Mark `json:"marks"`
		}{
			ExemplarID:   id,
			IsGTDAbsent:  true,
			IsRNPTAbsent: true,
			Marks:        []models.Mark{marks[i]},
		})
	}

	_, err = MakeOzonRequest(cab, "POST", "https://api-seller.ozon.ru/v6/fbs/posting/product/exemplar/set", request)
	if err != nil {
		return err
	}

	if err := UpdateMarkingStatus(cab.Key, postingNumber, productID, codes[:quantity]); err != nil {
		log.Printf("[WARNING] Ошибка обновления состояния маркировки: %v", err)
	}

	return nil
}
