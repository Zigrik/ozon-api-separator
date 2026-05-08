package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Zigrik/license-system/license"
	"github.com/joho/godotenv"
)

// ======================== СТРУКТУРЫ ========================

type CabinetConfig struct {
	Name     string
	ClientID string
	APIKey   string
	Color    string
	BgColor  string
	Key      string
}

type AppConfig struct {
	Password      string
	Cabinets      map[string]*CabinetConfig
	ActiveCabinet string
	AuthToken     string
}

type Posting struct {
	PostingNumber string         `json:"posting_number"`
	Status        string         `json:"status"`
	OrderID       int64          `json:"order_id"`
	CreatedAt     time.Time      `json:"created_at"`
	Products      []Product      `json:"products"`
	Requirements  *Requirements  `json:"requirements,omitempty"`
	AnalyticsData *AnalyticsData `json:"analytics_data,omitempty"`
	FinancialData *FinancialData `json:"financial_data,omitempty"`
	IsFolderReady bool           `json:"is_folder_ready"` // Флаг для серого выделения
}

type Product struct {
	SKU                int64       `json:"sku"`
	Name               string      `json:"name"`
	Quantity           int         `json:"quantity"`
	ProductID          int64       `json:"product_id,omitempty"`
	OfferID            string      `json:"offer_id,omitempty"`
	Price              interface{} `json:"price,omitempty"`
	IsMandatoryMarked  bool        `json:"is_mandatory_marked"`
	IsGtdRequired      bool        `json:"is_gtd_required"`
	IsCountryRequired  bool        `json:"is_country_required"`
	IsMarkingCompleted bool        `json:"is_marking_completed"`
}

type AnalyticsData struct {
	City string `json:"city,omitempty"`
}

type FinancialData struct {
	Products []FinancialProduct `json:"products"`
}

type FinancialProduct struct {
	ProductID int64       `json:"product_id"`
	Price     interface{} `json:"price"`
	Quantity  int         `json:"quantity"`
}

type Requirements struct {
	ProductsRequiringGTD           []int64 `json:"products_requiring_gtd,omitempty"`
	ProductsRequiringCountry       []int64 `json:"products_requiring_country,omitempty"`
	ProductsRequiringMandatoryMark []int64 `json:"products_requiring_mandatory_mark,omitempty"`
	ProductsRequiringRNPT          []int64 `json:"products_requiring_rnpt,omitempty"`
	ProductsRequiringJWUIN         []int64 `json:"products_requiring_jw_uin,omitempty"`
	ProductsRequiringChangeCountry []int64 `json:"products_requiring_change_country,omitempty"`
	ProductsRequiringImei          []int64 `json:"products_requiring_imei,omitempty"`
	ProductsRequiringWeight        []int64 `json:"products_requiring_weight,omitempty"`
}

type PostingsListResponse struct {
	Result struct {
		Postings []Posting `json:"postings"`
	} `json:"result"`
}

type PostingsFilter struct {
	Filter struct {
		Status     string     `json:"status,omitempty"`
		CutoffFrom *time.Time `json:"cutoff_from,omitempty"`
		CutoffTo   *time.Time `json:"cutoff_to,omitempty"`
	} `json:"filter"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

type ShipRequest struct {
	PostingNumber string        `json:"posting_number"`
	Packages      []ShipPackage `json:"packages"`
}

type ShipPackage struct {
	Products []ShipProduct `json:"products"`
}

type ShipProduct struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

type ShipResponse struct {
	AdditionalData []struct {
		PostingNumber string `json:"posting_number"`
		Products      []struct {
			CurrencyCode  string   `json:"currency_code"`
			MandatoryMark []string `json:"mandatory_mark"`
			Name          string   `json:"name"`
			OfferID       string   `json:"offer_id"`
			Price         string   `json:"price"`
			Quantity      int      `json:"quantity"`
			SKU           int64    `json:"sku"`
		} `json:"products"`
	} `json:"additional_data"`
	Result []string `json:"result"`
}

type CountryInfo struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type CountrySetRequest struct {
	PostingNumber  string `json:"posting_number"`
	ProductID      int64  `json:"product_id"`
	CountryISOCode string `json:"country_iso_code"`
}

type ExemplarCreateRequest struct {
	PostingNumber string `json:"posting_number"`
}

type ExemplarCreateResponse struct {
	PostingNumber string `json:"posting_number"`
	MultiBoxQty   int    `json:"multi_box_qty"`
	Products      []struct {
		ProductID               int64   `json:"product_id"`
		Quantity                int     `json:"quantity"`
		IsMandatoryMarkNeeded   bool    `json:"is_mandatory_mark_needed"`
		IsMandatoryMarkPossible bool    `json:"is_mandatory_mark_possible"`
		IsGTDNeeded             bool    `json:"is_gtd_needed"`
		IsRNPTNeeded            bool    `json:"is_rnpt_needed"`
		IsWeightNeeded          bool    `json:"is_weight_needed"`
		WeightMin               float64 `json:"weight_min"`
		WeightMax               float64 `json:"weight_max"`
		HasImei                 bool    `json:"has_imei"`
		IsJwUinNeeded           bool    `json:"is_jw_uin_needed"`
		Exemplars               []struct {
			ExemplarID   int64  `json:"exemplar_id"`
			GTD          string `json:"gtd"`
			IsGTDAbsent  bool   `json:"is_gtd_absent"`
			IsRNPTAbsent bool   `json:"is_rnpt_absent"`
			RNPT         string `json:"rnpt"`
			Weight       int    `json:"weight"`
			Marks        []struct {
				Mark     string `json:"mark"`
				MarkType string `json:"mark_type"`
			} `json:"marks"`
		} `json:"exemplars"`
	} `json:"products"`
}

type ExemplarStatusResponse struct {
	PostingNumber string `json:"posting_number"`
	Status        string `json:"status"`
	Products      []struct {
		ProductID int64 `json:"product_id"`
		Exemplars []struct {
			ExemplarID   int64  `json:"exemplar_id"`
			GTD          string `json:"gtd"`
			IsGTDAbsent  bool   `json:"is_gtd_absent"`
			IsRNPTAbsent bool   `json:"is_rnpt_absent"`
			RNPT         string `json:"rnpt"`
			Weight       int    `json:"weight"`
			Marks        []struct {
				Mark        string   `json:"mark"`
				MarkType    string   `json:"mark_type"`
				CheckStatus string   `json:"check_status"`
				ErrorCodes  []string `json:"error_codes"`
			} `json:"marks"`
			GTDCheckStatus    string   `json:"gtd_check_status"`
			GTDErrorCodes     []string `json:"gtd_error_codes"`
			RNPTCheckStatus   string   `json:"rnpt_check_status"`
			RNPTErrorCodes    []string `json:"rnpt_error_codes"`
			WeightCheckStatus string   `json:"weight_check_status"`
			WeightErrorCodes  []string `json:"weight_error_codes"`
		} `json:"exemplars"`
	} `json:"products"`
}

type CreateLabelRequest struct {
	PostingNumbers []string `json:"posting_number"`
}

type CreateLabelResponse struct {
	Result struct {
		Tasks []struct {
			TaskID   int64  `json:"task_id"`
			TaskType string `json:"task_type"`
		} `json:"tasks"`
	} `json:"result"`
}

type GetLabelRequest struct {
	TaskID int64 `json:"task_id"`
}

type GetLabelResponse struct {
	Result struct {
		Error   string `json:"error"`
		Status  string `json:"status"`
		FileURL string `json:"file_url"`
	} `json:"result"`
}

type MarkForUpdate struct {
	ProductID int64
	Quantity  int
	Code      string
}

type ActionLog struct {
	Timestamp    string   `json:"timestamp"`
	OrderNumber  string   `json:"order_number"`
	Products     []string `json:"products"`
	SubOrders    []string `json:"sub_orders"`
	Marks        []string `json:"marks"`
	LabelSaved   bool     `json:"label_saved"`
	Success      bool     `json:"success"`
	ErrorMessage string   `json:"error_message,omitempty"`
}

var appConfig *AppConfig
var markingCodes []string
var codesMutex sync.Mutex

var labelQueue = struct {
	sync.Mutex
	Jobs        map[string][]string
	Status      map[string]string
	Progress    map[string]int
	Total       map[string]int
	StartTime   map[string]time.Time
	Errors      map[string]string
	FailedItems map[string][]string
}{
	Jobs:        make(map[string][]string),
	Status:      make(map[string]string),
	Progress:    make(map[string]int),
	Total:       make(map[string]int),
	StartTime:   make(map[string]time.Time),
	Errors:      make(map[string]string),
	FailedItems: make(map[string][]string),
}

var labelGenerationMutex sync.Mutex
var isLabelGenerationRunning = false

var autoModeEnabled = false
var autoModeMutex sync.RWMutex
var monitorInterval = 30 * time.Second
var csvWorkerStop chan struct{}
var csvWorkerRunning = false

// ======================== ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ========================

func getClientIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		ips := strings.Split(ip, ",")
		ip = strings.TrimSpace(ips[0])
	}
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	if idx := strings.Index(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	ip = strings.TrimPrefix(ip, "[")
	ip = strings.TrimSuffix(ip, "]")
	if ip == "" || ip == "1" || ip == "::1" {
		ip = "127.0.0.1"
	}
	return ip
}

func logAction(r *http.Request, cabinetName string, action string) {
	ip := getClientIP(r)
	if cabinetName != "" {
		log.Printf("%s - %s - %s", ip, cabinetName, action)
	} else {
		log.Printf("%s - %s", ip, action)
	}
}

func loadMarkingCodes() error {
	log.Println("loadMarkingCodes: начало загрузки")

	codesMutex.Lock()
	defer codesMutex.Unlock()

	file, err := os.Open("GTINs.txt")
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("loadMarkingCodes: файл GTINs.txt не найден")
			return nil
		}
		log.Printf("loadMarkingCodes: ошибка открытия файла: %v", err)
		return err
	}
	defer file.Close()

	markingCodes = make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		code := strings.TrimSpace(scanner.Text())
		if code != "" {
			markingCodes = append(markingCodes, code)
		}
	}

	log.Printf("loadMarkingCodes: загружено %d кодов маркировки", len(markingCodes))
	return scanner.Err()
}

func saveMarkingCodes() error {
	file, err := os.Create("GTINs.txt")
	if err != nil {
		return err
	}
	defer file.Close()
	for _, code := range markingCodes {
		_, err := file.WriteString(code + "\n")
		if err != nil {
			return err
		}
	}
	return nil
}

func getMarkingCodes(count int) ([]string, error) {
	log.Printf("📦 Запрос %d кодов маркировки", count)

	codesMutex.Lock()
	defer codesMutex.Unlock()

	log.Printf("📊 Доступно кодов: %d", len(markingCodes))

	if len(markingCodes) < count {
		log.Printf("❌ Недостаточно кодов! Нужно %d, доступно %d", count, len(markingCodes))
		return nil, fmt.Errorf("недостаточно кодов: нужно %d, доступно %d", count, len(markingCodes))
	}

	codes := make([]string, count)
	for i := 0; i < count; i++ {
		codes[i] = markingCodes[i]
	}

	remaining := markingCodes[count:]
	markingCodes = append(remaining, codes...)

	log.Printf("✅ Использовано %d кодов, перемещено в конец очереди", count)

	if len(markingCodes) > 0 {
		previewCount := 5
		if len(markingCodes) < previewCount {
			previewCount = len(markingCodes)
		}
		log.Printf("📋 Первые %d кодов в очереди: %v", previewCount, markingCodes[:previewCount])
	}

	if err := saveMarkingCodes(); err != nil {
		log.Printf("❌ Ошибка сохранения: %v", err)
		return nil, fmt.Errorf("ошибка сохранения кодов: %w", err)
	}

	return codes, nil
}

func loadConfig() error {
	err := godotenv.Load()
	if err != nil {
		log.Println("Предупреждение: файл .env не найден")
	}
	password := os.Getenv("APP_PASSWORD")
	if password == "" {
		return fmt.Errorf("APP_PASSWORD не установлен")
	}
	appConfig = &AppConfig{
		Password:      password,
		Cabinets:      make(map[string]*CabinetConfig),
		ActiveCabinet: "shinorama",
		AuthToken:     os.Getenv("AUTH_TOKEN"),
	}
	cabinets := map[string]struct {
		Name    string
		Color   string
		BgColor string
	}{
		"shinorama":        {"Шинорама", "#2e7d32", "#e8f5e9"},
		"trecktrack":       {"TreckTrack", "#f57c00", "#fff9c4"},
		"sevenhundredshin": {"700shin", "#c62828", "#ffebee"},
	}
	for key, cabinet := range cabinets {
		envKey := strings.ToUpper(key)
		clientID := os.Getenv(envKey + "_CLIENT_ID")
		apiKey := os.Getenv(envKey + "_API_KEY")
		if clientID == "" || apiKey == "" {
			log.Printf("Внимание: неполная конфигурация для %s", cabinet.Name)
		}
		appConfig.Cabinets[key] = &CabinetConfig{
			Name:     cabinet.Name,
			ClientID: clientID,
			APIKey:   apiKey,
			Color:    cabinet.Color,
			BgColor:  cabinet.BgColor,
			Key:      key,
		}
	}
	return nil
}

func getActiveConfig() *CabinetConfig {
	return appConfig.Cabinets[appConfig.ActiveCabinet]
}

func makeOzonRequest(cabinet *CabinetConfig, method string, url string, body interface{}) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Client-Id", cabinet.ClientID)
	req.Header.Set("Api-Key", cabinet.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
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

func getExemplarStatus(cabinet *CabinetConfig, postingNumber string) (*ExemplarStatusResponse, error) {
	url := "https://api-seller.ozon.ru/v5/fbs/posting/product/exemplar/status"
	request := ExemplarCreateRequest{
		PostingNumber: postingNumber,
	}

	respBody, err := makeOzonRequest(cabinet, "POST", url, request)
	if err != nil {
		return nil, err
	}

	var response ExemplarStatusResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func getExemplarIDs(cabinet *CabinetConfig, postingNumber string) (*ExemplarCreateResponse, error) {
	url := "https://api-seller.ozon.ru/v6/fbs/posting/product/exemplar/create-or-get"
	request := ExemplarCreateRequest{
		PostingNumber: postingNumber,
	}

	respBody, err := makeOzonRequest(cabinet, "POST", url, request)
	if err != nil {
		return nil, err
	}

	var response ExemplarCreateResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

func getCountriesList(cabinet *CabinetConfig) ([]CountryInfo, error) {
	url := "https://api-seller.ozon.ru/v2/posting/fbs/product/country/list"
	respBody, err := makeOzonRequest(cabinet, "POST", url, map[string]interface{}{})
	if err != nil {
		log.Printf("Ошибка запроса списка стран: %v", err)
		return getDefaultCountries(), nil
	}
	var response struct {
		Result []struct {
			Name           string `json:"name"`
			CountryISOCode string `json:"country_iso_code"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		log.Printf("Ошибка парсинга списка стран: %v", err)
		return getDefaultCountries(), nil
	}
	countries := make([]CountryInfo, 0)
	for _, c := range response.Result {
		if c.Name != "" && c.CountryISOCode != "" {
			countries = append(countries, CountryInfo{
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

func getDefaultCountries() []CountryInfo {
	return []CountryInfo{
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

func setCountry(cabinet *CabinetConfig, postingNumber string, productID int64, countryCode string) error {
	url := "https://api-seller.ozon.ru/v2/posting/fbs/product/country/set"
	countryCode = strings.TrimSpace(strings.ToUpper(countryCode))
	request := CountrySetRequest{
		PostingNumber:  postingNumber,
		ProductID:      productID,
		CountryISOCode: countryCode,
	}
	_, err := makeOzonRequest(cabinet, "POST", url, request)
	return err
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Auth-Token")
		if token != "" && appConfig.AuthToken != "" && token == appConfig.AuthToken {
			next(w, r)
			return
		}
		password := r.Header.Get("X-Password")
		if password == appConfig.Password {
			next(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
	}
}

// ======================== API ОБРАБОТЧИКИ ========================

func handleCheckPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Password == appConfig.Password {
		logAction(r, "", "авторизация - успешно")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"token":  appConfig.AuthToken,
		})
	} else {
		logAction(r, "", "авторизация - неверный пароль")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid password"})
	}
}

func handleSwitchCabinet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Cabinet string `json:"cabinet"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cabinet, exists := appConfig.Cabinets[req.Cabinet]
	if !exists {
		http.Error(w, "Cabinet not found", http.StatusNotFound)
		return
	}
	appConfig.ActiveCabinet = req.Cabinet
	logAction(r, cabinet.Name, fmt.Sprintf("переключение на %s", cabinet.Name))
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "active": req.Cabinet})
}

func handleGetOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cabinet := getActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		logAction(r, cabinet.Name, "ошибка - кабинет не настроен")
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}
	logAction(r, cabinet.Name, "загрузка заказов")
	orders, err := getAwaitingPackagingOrders(cabinet)
	if err != nil {
		logAction(r, cabinet.Name, fmt.Sprintf("ошибка загрузки заказов: %v", err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logAction(r, cabinet.Name, fmt.Sprintf("загружено %d заказов", len(orders)))

	// Отправляем список заказов, включая статус папки
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"orders":  orders,
		"cabinet": cabinet.Name,
	})
}

func handleShipOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Orders []struct {
			PostingNumber string `json:"posting_number"`
			Products      []struct {
				ProductID int64 `json:"product_id"`
				Quantity  int   `json:"quantity"`
			} `json:"products"`
		} `json:"orders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cabinet := getActiveConfig()
	results := make([]map[string]interface{}, 0)
	logAction(r, cabinet.Name, fmt.Sprintf("начало отправки %d заказов", len(req.Orders)))

	for _, order := range req.Orders {
		packages := make([]ShipPackage, 0)
		for _, product := range order.Products {
			for i := 0; i < product.Quantity; i++ {
				packages = append(packages, ShipPackage{
					Products: []ShipProduct{
						{
							ProductID: product.ProductID,
							Quantity:  1,
						},
					},
				})
			}
		}

		result := map[string]interface{}{
			"posting_number": order.PostingNumber,
		}

		maxRetries := 3
		retryDelay := 1 * time.Second
		var shipments []string
		var err error

		for attempt := 1; attempt <= maxRetries; attempt++ {
			shipments, err = shipOrder(cabinet, order.PostingNumber, packages)
			if err == nil {
				break
			}
			log.Printf("⚠️ Попытка %d/%d: ошибка отправки %s: %v", attempt, maxRetries, order.PostingNumber, err)
			if attempt < maxRetries {
				time.Sleep(retryDelay)
			}
		}

		if err != nil {
			result["status"] = "error"
			result["error"] = err.Error()
			logAction(r, cabinet.Name, fmt.Sprintf("ошибка отправки %s после %d попыток: %v", order.PostingNumber, maxRetries, err))
		} else {
			result["status"] = "success"
			result["shipments"] = shipments
			result["message"] = fmt.Sprintf("Заказ %s разделен на %d отправлений", order.PostingNumber, len(packages))
			logAction(r, cabinet.Name, fmt.Sprintf("отправлен %s на %d упаковок", order.PostingNumber, len(packages)))
		}
		results = append(results, result)
	}

	successCount := 0
	errorCount := 0
	for _, r := range results {
		if r["status"] == "success" {
			successCount++
		} else {
			errorCount++
		}
	}
	logAction(r, cabinet.Name, fmt.Sprintf("отправка завершена: успешно %d, ошибок %d", successCount, errorCount))

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"results": results,
	})
}

func handleGetAvailableCodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	codesMutex.Lock()
	count := len(markingCodes)
	codesMutex.Unlock()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"count":  count,
	})
}

func handleAddMarkingsWithGTD(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PostingNumber string `json:"posting_number"`
		ProductID     int64  `json:"product_id"`
		Quantity      int    `json:"quantity"`
		GTDAbsent     bool   `json:"gtd_absent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Ошибка декодирования: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("📥 Запрос: posting=%s, product=%d, qty=%d", req.PostingNumber, req.ProductID, req.Quantity)

	cabinet := getActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("❌ Кабинет не настроен")
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	codes, err := getMarkingCodes(req.Quantity)
	if err != nil {
		log.Printf("❌ Коды: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}
	log.Printf("✅ Получено %d кодов", len(codes))

	exemplarData, err := getExemplarIDs(cabinet, req.PostingNumber)
	if err != nil {
		log.Printf("❌ Exemplar: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": fmt.Sprintf("Ошибка получения exemplar_id: %v", err),
		})
		return
	}

	var exemplarIDs []int64
	for _, p := range exemplarData.Products {
		if p.ProductID == req.ProductID {
			for _, e := range p.Exemplars {
				exemplarIDs = append(exemplarIDs, e.ExemplarID)
			}
			break
		}
	}

	if len(exemplarIDs) < req.Quantity {
		errMsg := fmt.Sprintf("Недостаточно exemplar_id: нужно %d, получено %d", req.Quantity, len(exemplarIDs))
		log.Printf("❌ %s", errMsg)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": errMsg,
		})
		return
	}

	type Mark struct {
		Mark     string `json:"mark"`
		MarkType string `json:"mark_type"`
	}

	type Exemplar struct {
		ExemplarID   int64  `json:"exemplar_id"`
		IsGTDAbsent  bool   `json:"is_gtd_absent"`
		IsRNPTAbsent bool   `json:"is_rnpt_absent"`
		Marks        []Mark `json:"marks"`
	}

	type ProductExemplar struct {
		ProductID int64      `json:"product_id"`
		Exemplars []Exemplar `json:"exemplars"`
	}

	request := struct {
		PostingNumber string            `json:"posting_number"`
		Products      []ProductExemplar `json:"products"`
	}{
		PostingNumber: req.PostingNumber,
		Products: []ProductExemplar{
			{
				ProductID: req.ProductID,
				Exemplars: make([]Exemplar, 0),
			},
		},
	}

	for i := 0; i < req.Quantity; i++ {
		exemplar := Exemplar{
			ExemplarID:   exemplarIDs[i],
			IsGTDAbsent:  req.GTDAbsent,
			IsRNPTAbsent: true,
			Marks: []Mark{
				{
					Mark:     codes[i],
					MarkType: "mandatory_mark",
				},
			},
		}
		request.Products[0].Exemplars = append(request.Products[0].Exemplars, exemplar)
	}

	url := "https://api-seller.ozon.ru/v6/fbs/posting/product/exemplar/set"
	log.Printf("📤 Ozon: установка маркировки для %d экземпляров", req.Quantity)

	_, err = makeOzonRequest(cabinet, "POST", url, request)
	if err != nil {
		log.Printf("❌ Ozon: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	log.Printf("✅ Успешно: добавлено %d кодов", req.Quantity)

	codesMutex.Lock()
	remaining := len(markingCodes)
	codesMutex.Unlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"message":   fmt.Sprintf("Добавлено %d кодов маркировки, ГТД отмечено как отсутствующее", req.Quantity),
		"remaining": remaining,
	})
}

func handleGetCountries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cabinet := getActiveConfig()
	countries, err := getCountriesList(cabinet)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"countries": countries,
	})
}

func handleSetCountry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		PostingNumber string `json:"posting_number"`
		ProductID     int64  `json:"product_id"`
		CountryCode   string `json:"country_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cabinet := getActiveConfig()
	err := setCountry(cabinet, req.PostingNumber, req.ProductID, req.CountryCode)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": fmt.Sprintf("Страна производителя установлена: %s", req.CountryCode),
	})
}

func handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	loadingText := os.Getenv("LOADING_TEXT")
	if loadingText == "" {
		loadingText = "Трудолюбивые ослики делят и сортируют ваши заказы..."
	}

	imagePath := "static/images/not_donkey.png"
	log.Printf("🔍 Проверка файла: %s", imagePath)

	customImage := ""
	if _, err := os.Stat(imagePath); err == nil {
		customImage = "not_donkey.png"
		log.Printf("✅ Найден файл картинки: %s", imagePath)
	} else {
		log.Printf("❌ Файл не найден: %s, ошибка: %v", imagePath, err)
	}

	log.Printf("📢 Настройки: loading_text='%s', custom_image='%s'", loadingText, customImage)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"loading_text": loadingText,
		"custom_image": customImage,
	})
}

// ======================== ЛОГИРОВАНИЕ ========================

func writeToLog(orderNumber string, products []string, subOrders []string, marks []string, labelSaved bool, success bool, errMsg string) {
	go func() {
		logDir := "logs"
		os.MkdirAll(logDir, 0755)
		today := time.Now().Format("2006-01-02")
		logFile := filepath.Join(logDir, fmt.Sprintf("%s.json", today))

		entry := ActionLog{
			Timestamp:    time.Now().Format(time.RFC3339),
			OrderNumber:  orderNumber,
			Products:     products,
			SubOrders:    subOrders,
			Marks:        marks,
			LabelSaved:   labelSaved,
			Success:      success,
			ErrorMessage: errMsg,
		}

		var logs []ActionLog
		if data, err := os.ReadFile(logFile); err == nil {
			json.Unmarshal(data, &logs)
		}
		logs = append(logs, entry)
		data, _ := json.MarshalIndent(logs, "", "  ")
		os.WriteFile(logFile, data, 0644)
	}()
}

// ======================== CSV И ФАЙЛОВЫЙ МОНИТОРИНГ ========================

func buildPackages(products []struct {
	ProductID int64
	Quantity  int
}) []ShipPackage {
	packages := make([]ShipPackage, 0)
	for _, p := range products {
		for i := 0; i < p.Quantity; i++ {
			packages = append(packages, ShipPackage{
				Products: []ShipProduct{{ProductID: p.ProductID, Quantity: 1}},
			})
		}
	}
	return packages
}

func shipOrder(cabinet *CabinetConfig, postingNumber string, packages []ShipPackage) ([]string, error) {
	url := "https://api-seller.ozon.ru/v4/posting/fbs/ship"
	request := ShipRequest{
		PostingNumber: postingNumber,
		Packages:      packages,
	}
	respBody, err := makeOzonRequest(cabinet, "POST", url, request)
	if err != nil {
		return nil, err
	}
	var response ShipResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, err
	}
	return response.Result, nil
}

func addMarkingsForOrder(cabinet *CabinetConfig, postingNumber string, productID int64, quantity int, codes []string) error {
	exemplarData, err := getExemplarIDs(cabinet, postingNumber)
	if err != nil {
		return err
	}
	var exemplarIDs []int64
	for _, p := range exemplarData.Products {
		if p.ProductID == productID {
			for _, e := range p.Exemplars {
				exemplarIDs = append(exemplarIDs, e.ExemplarID)
			}
			break
		}
	}
	if len(exemplarIDs) < quantity {
		return fmt.Errorf("недостаточно exemplar_id")
	}

	type Mark struct {
		Mark     string `json:"mark"`
		MarkType string `json:"mark_type"`
	}

	type Exemplar struct {
		ExemplarID   int64  `json:"exemplar_id"`
		IsGTDAbsent  bool   `json:"is_gtd_absent"`
		IsRNPTAbsent bool   `json:"is_rnpt_absent"`
		Marks        []Mark `json:"marks"`
	}

	type ProductExemplar struct {
		ProductID int64      `json:"product_id"`
		Exemplars []Exemplar `json:"exemplars"`
	}

	request := struct {
		PostingNumber string            `json:"posting_number"`
		Products      []ProductExemplar `json:"products"`
	}{
		PostingNumber: postingNumber,
		Products: []ProductExemplar{
			{
				ProductID: productID,
				Exemplars: make([]Exemplar, 0),
			},
		},
	}

	for i := 0; i < quantity; i++ {
		request.Products[0].Exemplars = append(request.Products[0].Exemplars, Exemplar{
			ExemplarID:   exemplarIDs[i],
			IsGTDAbsent:  true,
			IsRNPTAbsent: true,
			Marks: []Mark{
				{
					Mark:     codes[i],
					MarkType: "mandatory_mark",
				},
			},
		})
	}

	url := "https://api-seller.ozon.ru/v6/fbs/posting/product/exemplar/set"
	_, err = makeOzonRequest(cabinet, "POST", url, request)
	return err
}

func startLabelGeneration(cabinet *CabinetConfig, shipments []string) string {
	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	labelQueue.Lock()
	labelQueue.Jobs[jobID] = shipments
	labelQueue.Status[jobID] = "pending"
	labelQueue.Total[jobID] = len(shipments)
	labelQueue.Progress[jobID] = 0
	labelQueue.StartTime[jobID] = time.Now()
	labelQueue.Errors[jobID] = ""
	labelQueue.FailedItems[jobID] = []string{}
	labelQueue.Unlock()
	go processLabelJob(jobID)
	return jobID
}

func processLabelJob(jobID string) {
	labelGenerationMutex.Lock()
	isLabelGenerationRunning = true
	labelGenerationMutex.Unlock()

	defer func() {
		labelGenerationMutex.Lock()
		isLabelGenerationRunning = false
		labelGenerationMutex.Unlock()
	}()

	time.Sleep(5 * time.Second)

	labelQueue.Lock()
	postingNumbers := labelQueue.Jobs[jobID]
	labelQueue.Status[jobID] = "processing"
	labelQueue.Unlock()

	cabinet := getActiveConfig()
	labelsPath := os.Getenv("LABELS_PATH")
	if labelsPath == "" {
		labelsPath = "labels"
	}

	completed := 0
	hasError := false
	var lastError string
	failedList := make([]string, 0)

	for _, postingNumber := range postingNumbers {
		log.Printf("📦 Обработка заказа %s (%d/%d)", postingNumber, completed+1, len(postingNumbers))

		taskID, err := createLabelTask(cabinet, postingNumber)
		if err != nil {
			lastError = fmt.Sprintf("Ошибка создания задачи для %s: %v", postingNumber, err)
			log.Printf("❌ %s", lastError)
			hasError = true
			failedList = append(failedList, postingNumber)
			completed++
			labelQueue.Lock()
			labelQueue.Progress[jobID] = completed
			labelQueue.Errors[jobID] = lastError
			labelQueue.FailedItems[jobID] = failedList
			labelQueue.Unlock()
			continue
		}

		content, err := getLabelByTaskIDWithRetry(cabinet, taskID, postingNumber)
		if err != nil {
			lastError = fmt.Sprintf("Ошибка получения этикетки для %s: %v", postingNumber, err)
			log.Printf("❌ %s", lastError)
			hasError = true
			failedList = append(failedList, postingNumber)
			completed++
			labelQueue.Lock()
			labelQueue.Progress[jobID] = completed
			labelQueue.Errors[jobID] = lastError
			labelQueue.FailedItems[jobID] = failedList
			labelQueue.Unlock()
			continue
		}

		fileName := fmt.Sprintf("%s.pdf", postingNumber)

		parts := strings.Split(postingNumber, "-")
		folderName := strings.Join(parts[:len(parts)-1], "-")
		if folderName == "" {
			folderName = postingNumber
		}

		err = saveLabelToFile(labelsPath, folderName, fileName, content)
		if err != nil {
			lastError = fmt.Sprintf("Ошибка сохранения этикетки для %s: %v", postingNumber, err)
			log.Printf("❌ %s", lastError)
			hasError = true
			failedList = append(failedList, postingNumber)
		} else {
			log.Printf("✅ Этикетка сохранена: %s/%s", folderName, fileName)
		}

		completed++
		labelQueue.Lock()
		labelQueue.Progress[jobID] = completed
		if hasError {
			labelQueue.Errors[jobID] = lastError
			labelQueue.FailedItems[jobID] = failedList
		}
		labelQueue.Unlock()
	}

	labelQueue.Lock()
	if hasError {
		labelQueue.Status[jobID] = "error"
	} else {
		labelQueue.Status[jobID] = "completed"
	}
	labelQueue.Unlock()

	log.Printf("🎉 Завершена обработка задания %s: %d/%d этикеток", jobID, completed, len(postingNumbers))
}

func createLabelTask(cabinet *CabinetConfig, postingNumber string) (int64, error) {
	url := "https://api-seller.ozon.ru/v2/posting/fbs/package-label/create"

	request := CreateLabelRequest{
		PostingNumbers: []string{postingNumber},
	}

	respBody, err := makeOzonRequest(cabinet, "POST", url, request)
	if err != nil {
		return 0, err
	}

	var response CreateLabelResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return 0, err
	}

	if len(response.Result.Tasks) == 0 {
		return 0, fmt.Errorf("не получены задачи на этикетки")
	}

	for _, task := range response.Result.Tasks {
		if task.TaskType == "small_label" {
			log.Printf("✅ Создана задача для заказа %s: task_id=%d", postingNumber, task.TaskID)
			return task.TaskID, nil
		}
	}

	log.Printf("⚠️ Для заказа %s маленькая этикетка не найдена, берём первую: task_id=%d", postingNumber, response.Result.Tasks[0].TaskID)
	return response.Result.Tasks[0].TaskID, nil
}

func getLabelByTaskID(cabinet *CabinetConfig, taskID int64) ([]byte, error) {
	url := "https://api-seller.ozon.ru/v1/posting/fbs/package-label/get"

	request := GetLabelRequest{
		TaskID: taskID,
	}

	respBody, err := makeOzonRequest(cabinet, "POST", url, request)
	if err != nil {
		return nil, err
	}

	var response GetLabelResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, err
	}

	if response.Result.Status == "failed" {
		return nil, fmt.Errorf("ошибка получения этикетки: %s", response.Result.Error)
	}

	if response.Result.Status != "completed" {
		return nil, fmt.Errorf("этикетка ещё не готова, статус: %s", response.Result.Status)
	}

	if response.Result.FileURL == "" {
		return nil, fmt.Errorf("URL этикетки не получен")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(response.Result.FileURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("✅ Этикетка загружена: task_id=%d, size=%d bytes", taskID, len(content))

	return content, nil
}

func getLabelByTaskIDWithRetry(cabinet *CabinetConfig, taskID int64, postingNumber string) ([]byte, error) {
	maxRetries := 3
	retryDelay := 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("📥 Попытка %d: получение этикетки для заказа %s (task_id=%d)", attempt, postingNumber, taskID)

		content, err := getLabelByTaskID(cabinet, taskID)
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

func saveLabelToFile(basePath, folderName, fileName string, content []byte) error {
	fullPath := filepath.Join(basePath, folderName)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(fullPath, fileName)
	return os.WriteFile(filePath, content, 0644)
}

func checkCSVAndProcess(cabinet *CabinetConfig, orderNumber string, folderPath string) {
	csvPath := filepath.Join(folderPath, "marks.csv")
	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		return
	}

	file, err := os.Open(csvPath)
	if err != nil {
		log.Printf("❌ Ошибка открытия CSV для %s: %v", orderNumber, err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var marksList []struct {
		OfferID  string
		MarkCode string
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line) // сплит по любому whitespace (включая двойные пробелы)
		if len(parts) < 2 {
			log.Printf("⚠️ Пропущена некорректная строка в CSV: %s", line)
			continue
		}
		marksList = append(marksList, struct {
			OfferID  string
			MarkCode string
		}{OfferID: parts[0], MarkCode: parts[len(parts)-1]})
	}

	if len(marksList) == 0 {
		return
	}

	// Запрашиваем актуальный заказ из API
	orders, err := getAwaitingPackagingOrders(cabinet)
	if err != nil {
		log.Printf("❌ Ошибка получения списка заказов: %v", err)
		return
	}
	var targetOrder *Posting
	for i := range orders {
		if orders[i].PostingNumber == orderNumber {
			targetOrder = &orders[i]
			break
		}
	}
	if targetOrder == nil {
		log.Printf("⚠️ Заказ %s не найден в актуальном списке Ozon", orderNumber)
		return
	}

	// Сопоставляем марки с товарами по OfferID (артикул из CSV)
	marksForUpdate := make([]MarkForUpdate, 0)
	for _, m := range marksList {
		for _, prod := range targetOrder.Products {
			if prod.OfferID == m.OfferID {
				marksForUpdate = append(marksForUpdate, MarkForUpdate{
					ProductID: prod.ProductID,
					Quantity:  1,
					Code:      m.MarkCode,
				})
				break
			}
		}
	}
	if len(marksForUpdate) == 0 {
		log.Printf("⚠️ Не найдено соответствие марок товарам для заказа %s", orderNumber)
		return
	}

	// Вносим марки через API
	success := true
	errMsg := ""
	for _, mu := range marksForUpdate {
		if err := addMarkingsForOrder(cabinet, targetOrder.PostingNumber, mu.ProductID, 1, []string{mu.Code}); err != nil {
			success = false
			errMsg = err.Error()
			log.Printf("❌ Ошибка добавления марки %s для заказа %s: %v", mu.Code, orderNumber, err)
			break
		}
	}
	if !success {
		writeToLog(orderNumber, nil, nil, nil, false, false, errMsg)
		return
	}

	// Собираем товары для отправки
	var toShipProducts []struct {
		ProductID int64
		Quantity  int
	}
	for _, p := range targetOrder.Products {
		toShipProducts = append(toShipProducts, struct {
			ProductID int64
			Quantity  int
		}{ProductID: p.ProductID, Quantity: p.Quantity})
	}
	if len(toShipProducts) == 0 {
		log.Printf("⚠️ Нет товаров для отправки в заказе %s", orderNumber)
		return
	}

	// Разделяем заказ
	shipments, err := shipOrder(cabinet, targetOrder.PostingNumber, buildPackages(toShipProducts))
	if err != nil {
		writeToLog(orderNumber, nil, nil, nil, false, false, err.Error())
		return
	}

	productsLog := make([]string, len(targetOrder.Products))
	for i, p := range targetOrder.Products {
		productsLog[i] = fmt.Sprintf("%s (x%d)", p.OfferID, p.Quantity)
	}
	subOrdersLog := make([]string, len(shipments))
	for i, s := range shipments {
		subOrdersLog[i] = s
	}
	marksLog := make([]string, len(marksForUpdate))
	for i, m := range marksForUpdate {
		marksLog[i] = m.Code
	}
	writeToLog(orderNumber, productsLog, subOrdersLog, marksLog, false, true, "")

	// Заказываем этикетки
	startLabelGeneration(cabinet, shipments)

	// Удаляем CSV-файл
	os.Remove(csvPath)
}

func startCSVWorker(cabinet *CabinetConfig, labelsPath string) {
	if csvWorkerRunning {
		return
	}
	csvWorkerRunning = true
	csvWorkerStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(monitorInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !isAutoModeEnabled() {
					continue
				}
				entries, err := os.ReadDir(labelsPath)
				if err != nil {
					continue
				}
				for _, entry := range entries {
					if entry.IsDir() {
						orderNum := entry.Name()
						folderPath := filepath.Join(labelsPath, orderNum)
						csvPath := filepath.Join(folderPath, "marks.csv")
						if _, err := os.Stat(csvPath); err == nil {
							checkCSVAndProcess(cabinet, orderNum, folderPath)
						}
					}
				}
			case <-csvWorkerStop:
				csvWorkerRunning = false
				return
			}
		}
	}()
}

func stopCSVWorker() {
	if csvWorkerStop != nil {
		close(csvWorkerStop)
	}
	csvWorkerRunning = false
}

func setAutoMode(enabled bool) {
	autoModeMutex.Lock()
	defer autoModeMutex.Unlock()
	autoModeEnabled = enabled
}

func isAutoModeEnabled() bool {
	autoModeMutex.RLock()
	defer autoModeMutex.RUnlock()
	return autoModeEnabled
}

func handleToggleAutoMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	setAutoMode(req.Enabled)
	if req.Enabled {
		labelsPath := os.Getenv("LABELS_PATH")
		if labelsPath == "" {
			labelsPath = "labels"
		}
		startCSVWorker(getActiveConfig(), labelsPath)
	} else {
		stopCSVWorker()
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "enabled": req.Enabled})
}

func handleGetAutoModeStatus(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{"enabled": isAutoModeEnabled()})
}

func handleGetLabelStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("job_id")
	if jobID == "" {
		http.Error(w, "job_id required", http.StatusBadRequest)
		return
	}

	labelQueue.Lock()
	defer labelQueue.Unlock()

	status := labelQueue.Status[jobID]
	progress := labelQueue.Progress[jobID]
	total := labelQueue.Total[jobID]
	errMsg := labelQueue.Errors[jobID]
	failedItems := labelQueue.FailedItems[jobID]

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       status,
		"progress":     progress,
		"total":        total,
		"error":        errMsg,
		"failed_items": failedItems,
	})
}

func handleCancelLabelGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	labelGenerationMutex.Lock()
	if isLabelGenerationRunning {
		isLabelGenerationRunning = false
		labelGenerationMutex.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "ok",
			"message": "Генерация этикеток отменена",
		})
	} else {
		labelGenerationMutex.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "idle",
			"message": "Нет активных задач",
		})
	}
}

func handleRetryLabelGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	labelGenerationMutex.Lock()
	if isLabelGenerationRunning {
		labelGenerationMutex.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "busy",
			"message": "Уже выполняется генерация этикеток",
		})
		return
	}
	isLabelGenerationRunning = true
	labelGenerationMutex.Unlock()

	var req struct {
		JobID       string   `json:"job_id"`
		FailedItems []string `json:"failed_items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		labelGenerationMutex.Lock()
		isLabelGenerationRunning = false
		labelGenerationMutex.Unlock()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	newJobID := fmt.Sprintf("retry_%d", time.Now().UnixNano())

	labelQueue.Lock()
	labelQueue.Jobs[newJobID] = req.FailedItems
	labelQueue.Status[newJobID] = "pending"
	labelQueue.Total[newJobID] = len(req.FailedItems)
	labelQueue.Progress[newJobID] = 0
	labelQueue.StartTime[newJobID] = time.Now()
	labelQueue.Errors[newJobID] = ""
	labelQueue.FailedItems[newJobID] = []string{}
	labelQueue.Unlock()

	go processLabelJob(newJobID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"job_id": newJobID,
	})
}

func getAwaitingPackagingOrders(cabinet *CabinetConfig) ([]Posting, error) {
	url := "https://api-seller.ozon.ru/v3/posting/fbs/unfulfilled/list"
	now := time.Now()
	cutoffFrom := now.AddDate(0, 0, -30)
	cutoffTo := now.AddDate(0, 0, 7)
	filter := PostingsFilter{
		Limit:  1000,
		Offset: 0,
	}
	filter.Filter.Status = "awaiting_packaging"
	filter.Filter.CutoffFrom = &cutoffFrom
	filter.Filter.CutoffTo = &cutoffTo

	respBody, err := makeOzonRequest(cabinet, "POST", url, filter)
	if err != nil {
		return nil, err
	}

	var response PostingsListResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	// Обогащаем товары данными из financial_data и requirements
	for i := range response.Result.Postings {
		posting := &response.Result.Postings[i]

		// Создаем карту финансовых данных по SKU
		financialMap := make(map[int64]*FinancialProduct)
		if posting.FinancialData != nil {
			for idx := range posting.FinancialData.Products {
				fp := &posting.FinancialData.Products[idx]
				financialMap[fp.ProductID] = fp
			}
		}

		// Создаем карты требований
		markingMap := make(map[int64]bool)
		gtdMap := make(map[int64]bool)
		countryMap := make(map[int64]bool)

		if posting.Requirements != nil {
			for _, pid := range posting.Requirements.ProductsRequiringMandatoryMark {
				markingMap[pid] = true
			}
			for _, pid := range posting.Requirements.ProductsRequiringGTD {
				gtdMap[pid] = true
			}
			for _, pid := range posting.Requirements.ProductsRequiringCountry {
				countryMap[pid] = true
			}
		}

		// Обогащаем каждый товар
		for j := range posting.Products {
			product := &posting.Products[j]

			// Получаем ProductID и цену из financial_data
			if fp, ok := financialMap[product.SKU]; ok {
				product.ProductID = fp.ProductID
				switch v := fp.Price.(type) {
				case float64:
					product.Price = v
				case string:
					priceStr := strings.ReplaceAll(v, ",", ".")
					if price, err := strconv.ParseFloat(priceStr, 64); err == nil {
						product.Price = price
					}
				}
			} else {
				product.ProductID = product.SKU
			}

			// Устанавливаем требования
			product.IsMandatoryMarked = markingMap[product.ProductID] || markingMap[product.SKU]
			product.IsGtdRequired = gtdMap[product.ProductID] || gtdMap[product.SKU]
			product.IsCountryRequired = countryMap[product.ProductID] || countryMap[product.SKU]
		}
	}

	// Проверяем статус уже добавленных маркировок через метод /v5/fbs/posting/product/exemplar/status
	for i := range response.Result.Postings {
		posting := &response.Result.Postings[i]

		// Проверяем, есть ли в заказе товары, требующие маркировки или ГТД
		needsCheck := false
		for _, p := range posting.Products {
			if p.IsMandatoryMarked || p.IsGtdRequired {
				needsCheck = true
				break
			}
		}

		if needsCheck {
			log.Printf("🔍 Проверка статуса маркировки для заказа %s", posting.PostingNumber)

			statusResp, err := getExemplarStatus(cabinet, posting.PostingNumber)
			if err != nil {
				log.Printf("⚠️ Ошибка получения статуса маркировки для %s: %v", posting.PostingNumber, err)
				continue
			}

			if statusResp.Status == "ship_available" {
				log.Printf("✅ Заказ %s полностью готов к отправке (ship_available)", posting.PostingNumber)
				for j := range posting.Products {
					posting.Products[j].IsMandatoryMarked = false
					posting.Products[j].IsGtdRequired = false
					posting.Products[j].IsMarkingCompleted = true
				}
			} else if statusResp.Status == "update_available" {
				log.Printf("⚠️ Заказ %s требует добавления маркировки/ГТД (update_available)", posting.PostingNumber)
			} else if statusResp.Status == "validation_in_process" {
				log.Printf("⏳ Заказ %s на проверке (validation_in_process)", posting.PostingNumber)
			} else {
				log.Printf("ℹ️ Заказ %s статус: %s", posting.PostingNumber, statusResp.Status)
			}
		}
	}

	// Проверяем наличие папок для заказов (для авто-режима)
	labelsPath := os.Getenv("LABELS_PATH")
	if labelsPath == "" {
		labelsPath = "labels"
	}
	for i := range response.Result.Postings {
		posting := &response.Result.Postings[i]
		folderPath := filepath.Join(labelsPath, posting.PostingNumber)
		if _, err := os.Stat(folderPath); err == nil {
			posting.IsFolderReady = true
		}
	}

	return response.Result.Postings, nil
}

func getLocalIP() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "localhost"
	}
	addrs, err := net.LookupIP(hostname)
	if err == nil {
		for _, addr := range addrs {
			if ipv4 := addr.To4(); ipv4 != nil && !addr.IsLoopback() {
				if ipv4[0] == 169 && ipv4[1] == 254 {
					continue
				}
				return ipv4.String()
			}
		}
	}
	return "localhost"
}

func runApp() {
	if err := loadConfig(); err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}
	if err := loadMarkingCodes(); err != nil {
		log.Printf("Ошибка загрузки кодов маркировки: %v", err)
	}

	// Настройка интервала мониторинга из .env
	if intervalSec := os.Getenv("MONITOR_INTERVAL_SEC"); intervalSec != "" {
		if sec, err := strconv.Atoi(intervalSec); err == nil && sec > 0 {
			monitorInterval = time.Duration(sec) * time.Second
		}
	}
	// Автоматический режим при старте
	if auto, _ := strconv.ParseBool(os.Getenv("AUTO_MODE_ENABLED")); auto {
		setAutoMode(true)
		labelsPath := os.Getenv("LABELS_PATH")
		if labelsPath == "" {
			labelsPath = "labels"
		}
		startCSVWorker(getActiveConfig(), labelsPath)
	}

	os.MkdirAll("templates", 0755)
	os.MkdirAll("static", 0755)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "templates/index.html")
	})
	http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, r.URL.Path[1:])
	})

	http.HandleFunc("/api/check-password", handleCheckPassword)
	http.HandleFunc("/api/cabinet/switch", authMiddleware(handleSwitchCabinet))
	http.HandleFunc("/api/orders", authMiddleware(handleGetOrders))
	http.HandleFunc("/api/orders/ship", authMiddleware(handleShipOrders))
	http.HandleFunc("/api/codes/available", authMiddleware(handleGetAvailableCodes))
	http.HandleFunc("/api/markings/add-with-gtd", authMiddleware(handleAddMarkingsWithGTD))
	http.HandleFunc("/api/countries/list", authMiddleware(handleGetCountries))
	http.HandleFunc("/api/countries/set", authMiddleware(handleSetCountry))
	http.HandleFunc("/api/settings", handleGetSettings)
	http.HandleFunc("/api/labels/generate", authMiddleware(handleStartLabelGenerationForShipments))
	http.HandleFunc("/api/labels/status", authMiddleware(handleGetLabelStatus))
	http.HandleFunc("/api/labels/cancel", authMiddleware(handleCancelLabelGeneration))
	http.HandleFunc("/api/labels/retry", authMiddleware(handleRetryLabelGeneration))
	http.HandleFunc("/api/auto-mode/toggle", authMiddleware(handleToggleAutoMode))
	http.HandleFunc("/api/auto-mode/status", authMiddleware(handleGetAutoModeStatus))
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "static/favicon.ico") })

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		log.Printf("Предупреждение: PORT='%s' не является числом, используется порт 8080", port)
		port = "8080"
	}

	localIP := getLocalIP()
	log.Printf("Сервер запущен на http://%s:%s (http://localhost:%s)", localIP, port, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleStartLabelGenerationForShipments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	labelGenerationMutex.Lock()
	if isLabelGenerationRunning {
		labelGenerationMutex.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "busy",
			"message": "Уже выполняется генерация этикеток. Дождитесь завершения.",
		})
		return
	}
	isLabelGenerationRunning = true
	labelGenerationMutex.Unlock()

	var req struct {
		PostingNumbers []string `json:"posting_numbers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		labelGenerationMutex.Lock()
		isLabelGenerationRunning = false
		labelGenerationMutex.Unlock()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())

	labelQueue.Lock()
	if labelQueue.Jobs == nil {
		labelQueue.Jobs = make(map[string][]string)
	}
	if labelQueue.Status == nil {
		labelQueue.Status = make(map[string]string)
	}
	if labelQueue.Progress == nil {
		labelQueue.Progress = make(map[string]int)
	}
	if labelQueue.Total == nil {
		labelQueue.Total = make(map[string]int)
	}
	if labelQueue.StartTime == nil {
		labelQueue.StartTime = make(map[string]time.Time)
	}
	if labelQueue.Errors == nil {
		labelQueue.Errors = make(map[string]string)
	}
	if labelQueue.FailedItems == nil {
		labelQueue.FailedItems = make(map[string][]string)
	}

	labelQueue.Jobs[jobID] = req.PostingNumbers
	labelQueue.Status[jobID] = "pending"
	labelQueue.Progress[jobID] = 0
	labelQueue.Total[jobID] = len(req.PostingNumbers)
	labelQueue.StartTime[jobID] = time.Now()
	labelQueue.Errors[jobID] = ""
	labelQueue.FailedItems[jobID] = make([]string, 0)
	labelQueue.Unlock()

	go processLabelJob(jobID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"job_id": jobID,
	})
}

func main() {
	result := license.CheckLicense(
		"vGlxAZOhrJY+VjopJqaSQAc4e8zW9qAj2G5coWmQ3X4=",
		"license.key",
		"OZON Api Cabinet",
	)

	if !result.Valid {
		log.Fatalf("\n"+
			"═══════════════════════════════════════════════════\n"+
			"❌ ОШИБКА ЛИЦЕНЗИИ\n"+
			"%s\n"+
			"═══════════════════════════════════════════════════\n"+
			"📞 Обратитесь к администратору\n",
			result.Error)
	}

	log.Printf("✅ Лицензия активна. Компания: %s", result.Company)

	runApp()
}
