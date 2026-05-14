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

		parts := strings.Split(posting.PostingNumber, "-")
		prefix := strings.Join(parts[:len(parts)-1], "-")
		if _, err := os.Stat(filepath.Join(dataPath, prefix)); err == nil {
			posting.IsFolderReady = true
		}

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
				pid := posting.Products[j].ProductID
				posting.Products[j].IsMandatoryMarked = markMap[pid]
				posting.Products[j].IsGtdRequired = gtdMap[pid]
				posting.Products[j].IsCountryRequired = cntMap[pid]
			}
		}
	}
	return response.Result.Postings, nil
}

// GetOrdersByStatus - получает заказы по статусу (awaiting_packaging, awaiting_deliver, delivering, etc.)
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
