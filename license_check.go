package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Zigrik/license-system/license"
)

// LicenseCheckResult результат проверки для вашей программы
type LicenseCheckResult struct {
	Success bool   // true - лицензия валидна
	Company string // название компании (если валидна)
	Message string // сообщение для вывода (ошибка или успех)
}

// CheckProgramLicense проверяет лицензию для программы
// Параметры:
//   - decryptKeyPath: путь к файлу decrypt.key (обычно "decrypt.key")
//   - licenseKeyPath: путь к файлу лицензии (обычно "license.key")
//   - expectedProduct: ожидаемое название продукта (у вас "700")
//   - envCompanyKey: ключ в .env для названия компании (например "APP_COMPANY")
//
// Возвращает:
//   - LicenseCheckResult: результат проверки
func CheckProgramLicense(decryptKeyPath, licenseKeyPath, expectedProduct, envCompanyKey string) LicenseCheckResult {
	// 1. Получаем ожидаемую компанию из .env (если нужно)
	expectedCompany := getEnvValue(envCompanyKey)

	// 2. Вызываем функцию проверки из пакета license
	result := license.CheckLicense(decryptKeyPath, licenseKeyPath, expectedProduct)

	// 3. Если лицензия не валидна - возвращаем ошибку
	if !result.Valid {
		return LicenseCheckResult{
			Success: false,
			Company: "",
			Message: fmt.Sprintf("❌ %s", result.Error),
		}
	}

	// 4. Если указана компания в .env, проверяем соответствие
	if expectedCompany != "" && result.Company != expectedCompany {
		return LicenseCheckResult{
			Success: false,
			Company: "",
			Message: fmt.Sprintf("❌ Неверная компания\n   Ожидается: %s\n   В лицензии: %s", expectedCompany, result.Company),
		}
	}

	// 5. Всё хорошо
	return LicenseCheckResult{
		Success: true,
		Company: result.Company,
		Message: fmt.Sprintf("✅ Лицензия активна\n   Компания: %s\n   Продукт: %s", result.Company, expectedProduct),
	}
}

// getEnvValue читает значение из .env файла
func getEnvValue(key string) string {
	// Пробуем прочитать из переменной окружения (приоритет)
	if val := os.Getenv(key); val != "" {
		return val
	}

	// Читаем из файла .env
	file, err := os.Open(".env")
	if err != nil {
		return ""
	}
	defer file.Close()

	data, err := os.ReadFile(".env")
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		if strings.TrimSpace(parts[0]) == key {
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, `"'`)
			return value
		}
	}

	return ""
}
