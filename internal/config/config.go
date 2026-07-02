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

	// Глобальные пути (экспортируемые, с большой буквы)
	LabelsPath string // Единый путь для labels (признак готовности + сохранение этикеток)
	OrdersPath string // Путь для состояния заказов
	LogsPath   string // Путь для логов
)

func LoadConfig() error {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ Предупреждение: .env не найден, используются переменные окружения")
	}

	// Загружаем пути
	LabelsPath = getPath("LABELS_PATH", "labels")
	OrdersPath = getPath("ORDERS_PATH", "orders")
	LogsPath = getPath("LOGS_PATH", "logs")

	log.Printf("📁 Пути загружены из .env:")
	log.Printf("   labels (признак готовности + этикетки): %s", LabelsPath)
	log.Printf("   состояние заказов: %s", OrdersPath)
	log.Printf("   логи: %s", LogsPath)

	// Создаем папки
	os.MkdirAll(LabelsPath, 0755)
	os.MkdirAll(OrdersPath, 0755)
	os.MkdirAll(LogsPath, 0755)

	pwd := os.Getenv("APP_PASSWORD")
	if pwd == "" {
		return fmt.Errorf("APP_PASSWORD не установлен")
	}

	AppConfig = &models.AppConfig{
		Password:      pwd,
		Cabinets:      make(map[string]*models.CabinetConfig),
		ActiveCabinet: "shinorama",
		AuthToken:     os.Getenv("AUTH_TOKEN"),
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

		// Путь для сохранения этикеток для конкретного кабинета (внутри LabelsPath)
		cabinetLabelsPath := filepath.Join(LabelsPath, key)
		os.MkdirAll(cabinetLabelsPath, 0755)

		AppConfig.Cabinets[key] = &models.CabinetConfig{
			Name:     cab.Name,
			ClientID: clientID,
			APIKey:   apiKey,
			Key:      key,
			DataPath: cabinetLabelsPath,
			Color:    cab.Color,
			BgColor:  cab.BgColor,
		}

		if clientID == "" || apiKey == "" {
			log.Printf("⚠️ Кабинет '%s' не настроен (отсутствуют CLIENT_ID или API_KEY)", cab.Name)
		}
	}

	log.Printf("✅ Конфигурация загружена. Доступно кабинетов: %d", len(AppConfig.Cabinets))
	return nil
}

// getPath - возвращает путь с поддержкой абсолютных и относительных путей
func getPath(envKey, defaultPath string) string {
	path := os.Getenv(envKey)
	if path == "" {
		path = defaultPath
	}
	// Если путь абсолютный — не изменяем
	if filepath.IsAbs(path) {
		return path
	}
	// Иначе возвращаем как есть (относительный путь)
	return path
}

// GetLabelsPathForCabinet - возвращает путь к папке labels для конкретного кабинета
func GetLabelsPathForCabinet(cabinetKey string) string {
	return filepath.Join(LabelsPath, cabinetKey)
}

// GetLabelsSavePathForCabinet - возвращает путь для сохранения этикеток для кабинета
func GetLabelsSavePathForCabinet(cabinetKey string) string {
	if cab, ok := AppConfig.Cabinets[cabinetKey]; ok && cab.DataPath != "" {
		return cab.DataPath
	}
	return filepath.Join(LabelsPath, cabinetKey)
}

// GetOrdersPath - возвращает путь для файлов состояния
func GetOrdersPath() string {
	return OrdersPath
}

// GetLogsPath - возвращает путь для логов
func GetLogsPath() string {
	return LogsPath
}

func GetActiveConfig() *models.CabinetConfig {
	return AppConfig.Cabinets[AppConfig.ActiveCabinet]
}

func GetDataPathForCabinet(cabinetKey string) string {
	if cab, ok := AppConfig.Cabinets[cabinetKey]; ok && cab.DataPath != "" {
		return cab.DataPath
	}
	return filepath.Join(LabelsPath, cabinetKey)
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
	log.Println("📂 loadMarkingCodes: начало загрузки")
	CodesMutex.Lock()
	defer CodesMutex.Unlock()

	file, err := os.Open("GTINs.txt")
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("⚠️ loadMarkingCodes: файл GTINs.txt не найден")
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
	log.Printf("✅ loadMarkingCodes: загружено %d кодов маркировки", len(MarkingCodes))
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
	log.Printf("📦 Запрос %d кодов маркировки", count)
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
