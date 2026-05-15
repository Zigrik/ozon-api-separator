package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ozon-api-separator/internal/models"
)

type ShipPackage = models.ShipPackage
type ShipProduct = models.ShipProduct

// Разделение заказа (общее для авто-режима и кнопки)
func ShipOrder(cab *models.CabinetConfig, postingNumber string, packages []ShipPackage) ([]string, error) {
	url := "https://api-seller.ozon.ru/v4/posting/fbs/ship"
	body, err := MakeOzonRequest(cab, "POST", url, models.ShipRequest{
		PostingNumber: postingNumber,
		Packages:      packages,
	})
	if err != nil {
		return nil, err
	}
	var resp models.ShipResponse
	json.Unmarshal(body, &resp)
	return resp.Result, nil
}

// Получение заказов в статусе awaiting_packaging
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
		return nil, err
	}
	var response models.PostingsListResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("ошибка парсинга: %w", err)
	}

	dataPath := cab.DataPath
	if dataPath == "" {
		dataPath = filepath.Join("data", cab.Key)
	}

	for i := range response.Result.Postings {
		posting := &response.Result.Postings[i]

		// Проверка наличия папки
		parts := strings.Split(posting.PostingNumber, "-")
		prefix := strings.Join(parts[:len(parts)-1], "-")
		if _, err := os.Stat(filepath.Join(dataPath, prefix)); err == nil {
			posting.IsFolderReady = true
		}

		// Обогащение финансовыми данными
		if posting.FinancialData != nil {
			finMap := make(map[int64]float64)
			for _, fp := range posting.FinancialData.Products {
				if p, ok := fp.Price.(float64); ok {
					finMap[fp.ProductID] = p
				} else if s, ok := fp.Price.(string); ok {
					p, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64)
					finMap[fp.ProductID] = p
				}
			}
			for j := range posting.Products {
				if price, ok := finMap[posting.Products[j].ProductID]; ok {
					posting.Products[j].Price = price
				}
			}
		}

		// Обогащение требованиями
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

	// Проверка статуса уже добавленных маркировок
	for i := range response.Result.Postings {
		posting := &response.Result.Postings[i]

		needsCheck := false
		for _, p := range posting.Products {
			if p.IsMandatoryMarked || p.IsGtdRequired {
				needsCheck = true
				break
			}
		}

		if !needsCheck {
			continue
		}

		statusResp, err := getExemplarStatus(cab, posting.PostingNumber)
		if err != nil {
			continue
		}

		if statusResp.Status == "ship_available" {
			for j := range posting.Products {
				posting.Products[j].IsMarkingCompleted = true
				posting.Products[j].IsMandatoryMarked = false
				posting.Products[j].IsGtdRequired = false
			}
		}
	}

	return response.Result.Postings, nil
}

// Получение статуса маркировки
func getExemplarStatus(cab *models.CabinetConfig, postingNumber string) (*models.ExemplarStatusResponse, error) {
	url := "https://api-seller.ozon.ru/v5/fbs/posting/product/exemplar/status"
	request := models.ExemplarCreateRequest{
		PostingNumber: postingNumber,
	}

	respBody, err := MakeOzonRequest(cab, "POST", url, request)
	if err != nil {
		return nil, err
	}

	var response models.ExemplarStatusResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// Получение заказов по статусу (awaiting_deliver и др.)
func GetOrdersByStatus(cab *models.CabinetConfig, status string) ([]models.Posting, error) {
	url := "https://api-seller.ozon.ru/v3/posting/fbs/unfulfilled/list"
	now := time.Now()
	cutoffFrom := now.AddDate(0, 0, -30)
	cutoffTo := now.AddDate(0, 0, 7)
	filter := models.PostingsFilter{
		Limit:  1000,
		Offset: 0,
	}
	filter.Filter.Status = status
	filter.Filter.CutoffFrom = &cutoffFrom
	filter.Filter.CutoffTo = &cutoffTo

	respBody, err := MakeOzonRequest(cab, "POST", url, filter)
	if err != nil {
		return nil, err
	}
	var response models.PostingsListResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("ошибка парсинга: %w", err)
	}
	return response.Result.Postings, nil
}
