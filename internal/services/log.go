package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"ozon-api-separator/internal/models"
)

func WriteToLog(orderNumber string, products []string, subOrders []string, marks []string, labelSaved bool, success bool, errMsg string) {
	go func() {
		logDir := "logs"
		os.MkdirAll(logDir, 0755)
		today := time.Now().Format("2006-01-02")
		logFile := filepath.Join(logDir, fmt.Sprintf("%s.json", today))

		entry := models.ActionLog{
			Timestamp:    time.Now().Format(time.RFC3339),
			OrderNumber:  orderNumber,
			Products:     products,
			SubOrders:    subOrders,
			Marks:        marks,
			LabelSaved:   labelSaved,
			Success:      success,
			ErrorMessage: errMsg,
		}

		var logs []models.ActionLog
		if data, err := os.ReadFile(logFile); err == nil {
			json.Unmarshal(data, &logs)
		}
		logs = append(logs, entry)
		data, _ := json.MarshalIndent(logs, "", "  ")
		os.WriteFile(logFile, data, 0644)
	}()
}
