package services

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"ozon-api-separator/internal/config"
)

var (
	logger     *log.Logger
	logFile    *os.File
	loggerOnce sync.Once
)

// InitLogger - инициализирует логгер с записью в файл
func InitLogger() error {
	var err error
	loggerOnce.Do(func() {
		logsPath := config.GetLogsPath()
		// Создаем папку рекурсивно
		if err = os.MkdirAll(logsPath, 0755); err != nil {
			log.Printf("[ERROR] Не удалось создать папку логов %s: %v", logsPath, err)
			return
		}

		dateStr := time.Now().Format("2006-01-02")
		logFilePath := filepath.Join(logsPath, fmt.Sprintf("%s.log", dateStr))

		logFile, err = os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Printf("[ERROR] Не удалось создать файл лога %s: %v", logFilePath, err)
			return
		}

		// Пишем и в консоль, и в файл
		multiWriter := io.MultiWriter(os.Stdout, logFile)
		logger = log.New(multiWriter, "", log.LstdFlags)
	})

	return err
}

// CloseLogger - закрывает файл лога
func CloseLogger() {
	if logFile != nil {
		logFile.Close()
	}
}

// GetLogger - возвращает логгер
func GetLogger() *log.Logger {
	if logger == nil {
		InitLogger()
	}
	return logger
}

// LogInfo - логирует информационное сообщение
func LogInfo(format string, v ...interface{}) {
	GetLogger().Printf("[INFO] "+format, v...)
}

// LogWarning - логирует предупреждение
func LogWarning(format string, v ...interface{}) {
	GetLogger().Printf("[WARNING] "+format, v...)
}

// LogError - логирует ошибку
func LogError(format string, v ...interface{}) {
	GetLogger().Printf("[ERROR] "+format, v...)
}

// LogDebug - логирует отладочное сообщение
func LogDebug(format string, v ...interface{}) {
	GetLogger().Printf("[DEBUG] "+format, v...)
}
