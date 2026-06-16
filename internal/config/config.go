package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"ozon-api-separator/internal/models"

	"github.com/joho/godotenv"
)

// AppConfig - глобальная конфигурация приложения
var AppConfig *models.AppConfig

// LoadConfig - загружает конфигурацию из .env файла
func LoadConfig() error {
	if err := godotenv.Load(); err != nil {
		log.Println("Предупреждение: .env не найден, используются системные переменные окружения")
	}

	pwd := os.Getenv("APP_PASSWORD")
	if pwd == "" {
		return fmt.Errorf("APP_PASSWORD не установлен в .env файле")
	}

	AppConfig = &models.AppConfig{
		Password:      pwd,
		Cabinets:      make(map[string]*models.CabinetConfig),
		ActiveCabinet: "shinorama",
		AuthToken:     os.Getenv("AUTH_TOKEN"),
	}

	cabinets := map[string]string{
		"shinorama":        "Шинорама",
		"trecktrack":       "TreckTrack",
		"sevenhundredshin": "700shin",
	}

	for key, name := range cabinets {
		envKey := strings.ToUpper(key)
		clientID := os.Getenv(envKey + "_CLIENT_ID")
		apiKey := os.Getenv(envKey + "_API_KEY")

		// Путь для сохранения этикеток
		dataPath := os.Getenv(envKey + "_DATA_PATH")
		if dataPath == "" {
			dataPath = filepath.Join("data", key)
		}

		if clientID == "" || apiKey == "" {
			log.Printf("⚠️ Кабинет %s не настроен (отсутствуют переменные %s_CLIENT_ID или %s_API_KEY)",
				name, envKey, envKey)
		}

		AppConfig.Cabinets[key] = &models.CabinetConfig{
			Name:     name,
			ClientID: clientID,
			APIKey:   apiKey,
			Key:      key,
			DataPath: dataPath, // <-- ДОБАВЛЕНО
		}

		// Создаем папку для этикеток
		os.MkdirAll(dataPath, 0755)
	}

	log.Printf("✅ Конфигурация загружена. Доступно кабинетов: %d", len(AppConfig.Cabinets))
	return nil
}

// GetActiveConfig - возвращает конфигурацию активного кабинета
func GetActiveConfig() *models.CabinetConfig {
	return AppConfig.Cabinets[AppConfig.ActiveCabinet]
}
