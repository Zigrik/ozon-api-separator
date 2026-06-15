package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

	client := &http.Client{Timeout: 10 * time.Second}
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
