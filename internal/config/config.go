package config

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ozon-api-separator/internal/models"

	"github.com/joho/godotenv"
)

var (
	AppConfig                *models.AppConfig
	MarkingCodes             []string
	CodesMutex               sync.Mutex
	LabelGenerationMutex     sync.Mutex
	IsLabelGenerationRunning = false
	MonitorInterval          = 30 * time.Second

	AutoModePerCabinet   = make(map[string]bool)
	AutoModeCabinetMutex sync.RWMutex
	CSVWorkerStopChan    = make(map[string]chan struct{})
	CSVWorkerRunningMap  = make(map[string]bool)
	CSVWorkersMutex      sync.Mutex

	// Глобальные пути
	GlobalLabelsPath string
	GlobalOrdersPath string
	GlobalLogsPath   string
)

func LoadConfig() error {
	if err := godotenv.Load(); err != nil {
		log.Println("[WARNING] .env не найден, используются переменные окружения")
	}

	// Загружаем глобальные пути
	GlobalLabelsPath = getPath("LABELS_PATH", "labels")
	GlobalOrdersPath = getPath("ORDERS_PATH", "orders")
	GlobalLogsPath = getPath("LOGS_PATH", "logs")

	log.Printf("[INFO] Глобальные пути:")
	log.Printf("   labels: %s", GlobalLabelsPath)
	log.Printf("   orders: %s", GlobalOrdersPath)
	log.Printf("   logs: %s", GlobalLogsPath)

	// Создаем глобальные папки
	os.MkdirAll(GlobalLabelsPath, 0755)
	os.MkdirAll(GlobalOrdersPath, 0755)
	os.MkdirAll(GlobalLogsPath, 0755)

	pwd := os.Getenv("APP_PASSWORD")
	if pwd == "" {
		return fmt.Errorf("APP_PASSWORD не установлен")
	}

	AppConfig = &models.AppConfig{
		Password:      pwd,
		Cabinets:      make(map[string]*models.CabinetConfig),
		ActiveCabinet: os.Getenv("ACTIVE_CABINET"),
		AuthToken:     os.Getenv("AUTH_TOKEN"),
	}

	if AppConfig.ActiveCabinet == "" {
		AppConfig.ActiveCabinet = "shinorama"
	}

	cabinets := map[string]struct {
		Name    string
		Color   string
		BgColor string
		Key     string
	}{
		"shinorama":        {"Шинорама", "#2e7d32", "#e8f5e9", "shinorama"},
		"trecktrack":       {"TreckTrack", "#f57c00", "#fff9c4", "trecktrack"},
		"sevenhundredshin": {"700shin", "#c62828", "#ffebee", "sevenhundredshin"},
	}

	for key, cab := range cabinets {
		envKey := strings.ToUpper(key)
		clientID := os.Getenv(envKey + "_CLIENT_ID")
		apiKey := os.Getenv(envKey + "_API_KEY")

		// Получаем путь для labels кабинета (если не указан - используем глобальный)
		labelsPath := getCabinetLabelsPath(key, GlobalLabelsPath)

		// Создаем папку для кабинета
		os.MkdirAll(labelsPath, 0755)

		log.Printf("[INFO] Кабинет '%s':", cab.Name)
		log.Printf("   labels: %s", labelsPath)

		AppConfig.Cabinets[key] = &models.CabinetConfig{
			Name:       cab.Name,
			ClientID:   clientID,
			APIKey:     apiKey,
			Key:        key,
			LabelsPath: labelsPath,
			Color:      cab.Color,
			BgColor:    cab.BgColor,
		}

		if clientID == "" || apiKey == "" {
			log.Printf("[WARNING] Кабинет '%s' не настроен (отсутствуют CLIENT_ID или API_KEY)", cab.Name)
		}
	}

	log.Printf("[INFO] Конфигурация загружена. Доступно кабинетов: %d", len(AppConfig.Cabinets))
	return nil
}

// getPath - возвращает путь с поддержкой абсолютных и относительных путей
func getPath(envKey, defaultPath string) string {
	path := os.Getenv(envKey)
	if path == "" {
		path = defaultPath
	}
	return path
}

// getCabinetLabelsPath - возвращает путь для labels кабинета
func getCabinetLabelsPath(cabinetKey, defaultPath string) string {
	envKey := strings.ToUpper(cabinetKey) + "_LABELS_PATH"
	path := os.Getenv(envKey)
	if path == "" {
		path = filepath.Join(defaultPath, cabinetKey)
	}
	return path
}

// GetLabelsPathForCabinet - возвращает путь к папке labels для конкретного кабинета
func GetLabelsPathForCabinet(cabinetKey string) string {
	if cab, ok := AppConfig.Cabinets[cabinetKey]; ok && cab.LabelsPath != "" {
		return cab.LabelsPath
	}
	return filepath.Join(GlobalLabelsPath, cabinetKey)
}

// GetOrdersPath - возвращает глобальный путь для файлов состояния
func GetOrdersPath() string {
	return GlobalOrdersPath
}

// GetLogsPath - возвращает глобальный путь для логов
func GetLogsPath() string {
	return GlobalLogsPath
}

// GetDataPathForCabinet - возвращает путь для сохранения этикеток для кабинета
func GetDataPathForCabinet(cabinetKey string) string {
	return GetLabelsPathForCabinet(cabinetKey)
}

// GetWarehouseUN - возвращает ID склада "Ун." из .env
func GetWarehouseUN() int64 {
	val := os.Getenv("WAREHOUSE_UN")
	if val == "" {
		return 0
	}
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		log.Printf("[WARNING] Ошибка парсинга WAREHOUSE_UN=%s: %v", val, err)
		return 0
	}
	return id
}

// GetWarehouseREV - возвращает ID склада "Рев." из .env
func GetWarehouseREV() int64 {
	val := os.Getenv("WAREHOUSE_REV")
	if val == "" {
		return 0
	}
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		log.Printf("[WARNING] Ошибка парсинга WAREHOUSE_REV=%s: %v", val, err)
		return 0
	}
	return id
}

// GetWarehouseUNName - возвращает название склада "Ун." из .env
func GetWarehouseUNName() string {
	name := os.Getenv("WAREHOUSE_UN_NAME")
	if name == "" {
		return "Ун."
	}
	return name
}

// GetWarehouseREVName - возвращает название склада "Рев." из .env
func GetWarehouseREVName() string {
	name := os.Getenv("WAREHOUSE_REV_NAME")
	if name == "" {
		return "Рев."
	}
	return name
}

func GetActiveConfig() *models.CabinetConfig {
	return AppConfig.Cabinets[AppConfig.ActiveCabinet]
}

func SetAutoModeForCabinet(cabinetKey string, enabled bool) {
	AutoModeCabinetMutex.Lock()
	defer AutoModeCabinetMutex.Unlock()
	AutoModePerCabinet[cabinetKey] = enabled
}

func IsAutoModeEnabledForCabinet(cabinetKey string) bool {
	AutoModeCabinetMutex.RLock()
	defer AutoModeCabinetMutex.RUnlock()
	return AutoModePerCabinet[cabinetKey]
}

func GetAutoModeStatusForAllCabinets() map[string]bool {
	AutoModeCabinetMutex.RLock()
	defer AutoModeCabinetMutex.RUnlock()
	result := make(map[string]bool)
	for k, v := range AutoModePerCabinet {
		result[k] = v
	}
	return result
}

func LoadAutoModeSettings() {
	for key := range AppConfig.Cabinets {
		envKey := strings.ToUpper(key) + "_AUTO_MODE"
		if auto, _ := strconv.ParseBool(os.Getenv(envKey)); auto {
			SetAutoModeForCabinet(key, true)
		}
	}
}

func LoadMarkingCodes() error {
	log.Println("[INFO] loadMarkingCodes: начало загрузки")
	CodesMutex.Lock()
	defer CodesMutex.Unlock()

	file, err := os.Open("GTINs.txt")
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("[WARNING] loadMarkingCodes: файл GTINs.txt не найден")
			return nil
		}
		return err
	}
	defer file.Close()

	MarkingCodes = make([]string, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		code := strings.TrimSpace(scanner.Text())
		if code != "" {
			MarkingCodes = append(MarkingCodes, code)
		}
	}
	log.Printf("[INFO] loadMarkingCodes: загружено %d кодов маркировки", len(MarkingCodes))
	return scanner.Err()
}

func SaveMarkingCodes() error {
	file, err := os.Create("GTINs.txt")
	if err != nil {
		return err
	}
	defer file.Close()
	for _, code := range MarkingCodes {
		if _, err := file.WriteString(code + "\n"); err != nil {
			return err
		}
	}
	return nil
}

func GetMarkingCodes(count int) ([]string, error) {
	log.Printf("[INFO] Запрос %d кодов маркировки", count)
	CodesMutex.Lock()
	defer CodesMutex.Unlock()
	if len(MarkingCodes) < count {
		return nil, fmt.Errorf("недостаточно кодов: нужно %d, доступно %d", count, len(MarkingCodes))
	}
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		codes[i] = MarkingCodes[i]
	}
	remaining := MarkingCodes[count:]
	MarkingCodes = append(remaining, codes...)
	if err := SaveMarkingCodes(); err != nil {
		return nil, err
	}
	return codes, nil
}
