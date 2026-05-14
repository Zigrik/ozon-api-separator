package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ozon-api-separator/internal/models"
)

func MakeOzonRequest(cab *models.CabinetConfig, method, url string, body interface{}) ([]byte, error) {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, url, bytes.NewBuffer(data))
	req.Header.Set("Client-Id", cab.ClientID)
	req.Header.Set("Api-Key", cab.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("статус %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func GetExemplarIDs(cab *models.CabinetConfig, postingNumber string) (*models.ExemplarCreateResponse, error) {
	body, err := MakeOzonRequest(cab, "POST",
		"https://api-seller.ozon.ru/v6/fbs/posting/product/exemplar/create-or-get",
		models.ExemplarCreateRequest{PostingNumber: postingNumber})
	if err != nil {
		return nil, err
	}
	var resp models.ExemplarCreateResponse
	json.Unmarshal(body, &resp)
	return &resp, nil
}

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

func SetCountry(cab *models.CabinetConfig, postingNumber string, productID int64, countryCode string) error {
	url := "https://api-seller.ozon.ru/v2/posting/fbs/product/country/set"
	countryCode = strings.TrimSpace(strings.ToUpper(countryCode))
	request := models.CountrySetRequest{
		PostingNumber:  postingNumber,
		ProductID:      productID,
		CountryISOCode: countryCode,
	}
	_, err := MakeOzonRequest(cab, "POST", url, request)
	return err
}
