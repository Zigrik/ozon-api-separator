package config

import (
	"fmt"
	"log"
	"os"
	"strings"

	"ozon-api-separator/internal/models"

	"github.com/joho/godotenv"
)

// AppConfig - глобальная конфигурация приложения (доступна из любого пакета)
var AppConfig *models.AppConfig

// LoadConfig - загружает конфигурацию из .env файла
// Возвращает ошибку, если не установлен APP_PASSWORD
func LoadConfig() error {
	// Загружаем переменные из .env файла (если он существует)
	if err := godotenv.Load(); err != nil {
		log.Println("Предупреждение: .env не найден, используются системные переменные окружения")
	}

	// Проверяем наличие обязательного пароля
	pwd := os.Getenv("APP_PASSWORD")
	if pwd == "" {
		return fmt.Errorf("APP_PASSWORD не установлен в .env файле")
	}

	// Инициализируем структуру конфигурации
	AppConfig = &models.AppConfig{
		Password:      pwd,
		Cabinets:      make(map[string]*models.CabinetConfig),
		ActiveCabinet: "shinorama", // По умолчанию активен кабинет Шинорама
		AuthToken:     os.Getenv("AUTH_TOKEN"),
	}

	// Список поддерживаемых кабинетов
	cabinets := map[string]string{
		"shinorama":        "Шинорама",
		"trecktrack":       "TreckTrack",
		"sevenhundredshin": "700shin",
	}

	// Загружаем настройки каждого кабинета из переменных окружения
	for key, name := range cabinets {
		envKey := strings.ToUpper(key) // Преобразуем ключ в верхний регистр (SHINORAMA_CLIENT_ID)
		clientID := os.Getenv(envKey + "_CLIENT_ID")
		apiKey := os.Getenv(envKey + "_API_KEY")

		// Логируем предупреждение, если кабинет не настроен
		if clientID == "" || apiKey == "" {
			log.Printf("⚠️ Кабинет %s не настроен (отсутствуют переменные %s_CLIENT_ID или %s_API_KEY)",
				name, envKey, envKey)
		}

		// Сохраняем конфигурацию кабинета
		AppConfig.Cabinets[key] = &models.CabinetConfig{
			Name:     name,
			ClientID: clientID,
			APIKey:   apiKey,
			Key:      key,
		}
	}

	log.Printf("✅ Конфигурация загружена. Доступно кабинетов: %d", len(AppConfig.Cabinets))
	return nil
}

// GetActiveConfig - возвращает конфигурацию активного в данный момент кабинета
func GetActiveConfig() *models.CabinetConfig {
	return AppConfig.Cabinets[AppConfig.ActiveCabinet]
}
