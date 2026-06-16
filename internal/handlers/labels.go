package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

// HandleCreateLabels - обработчик заказа этикеток
// Метод: POST
// Тело запроса: {"posting_numbers": ["50573134-0085-1-1", "50573134-0085-1-2"]}
// Ответ: {"status": "ok", "task_id": 123456789, "message": "Задача создана"}
func HandleCreateLabels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PostingNumbers []string `json:"posting_numbers"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("❌ Ошибка декодирования запроса: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(req.PostingNumbers) == 0 {
		http.Error(w, "posting_numbers is required", http.StatusBadRequest)
		return
	}

	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("❌ Кабинет '%s' не настроен", cabinet.Name)
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	log.Printf("🏷️ Заказ этикеток для %d заказов", len(req.PostingNumbers))

	taskID, err := services.CreateLabelTask(cabinet, req.PostingNumbers)
	if err != nil {
		log.Printf("❌ Ошибка создания задачи для этикеток: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	log.Printf("✅ Создана задача для этикеток: task_id=%d", taskID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"task_id": taskID,
		"message": fmt.Sprintf("Задача для этикеток создана, ID: %d", taskID),
	})
}

// HandleGetLabelStatus - обработчик получения статуса этикетки
// Метод: GET
// Параметры: ?task_id=123456789
// Ответ: {"status": "ok", "label_status": "completed", "is_ready": true}
func HandleGetLabelStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskIDStr := r.URL.Query().Get("task_id")
	if taskIDStr == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid task_id", http.StatusBadRequest)
		return
	}

	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("❌ Кабинет '%s' не настроен", cabinet.Name)
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	status, err := services.GetLabelStatus(cabinet, taskID)
	if err != nil {
		log.Printf("❌ Ошибка получения статуса этикетки: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	isReady := status == "completed"

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"label_status": status,
		"is_ready":     isReady,
	})
}

// HandleDownloadLabel - обработчик скачивания этикетки
// Метод: GET
// Параметры: ?task_id=123456789&posting_number=50573134-0085-1-1
// Ответ: PDF файл
func HandleDownloadLabel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	taskIDStr := r.URL.Query().Get("task_id")
	postingNumber := r.URL.Query().Get("posting_number")

	if taskIDStr == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}
	if postingNumber == "" {
		http.Error(w, "posting_number is required", http.StatusBadRequest)
		return
	}

	taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid task_id", http.StatusBadRequest)
		return
	}

	cabinet := config.GetActiveConfig()
	if cabinet.ClientID == "" || cabinet.APIKey == "" {
		log.Printf("❌ Кабинет '%s' не настроен", cabinet.Name)
		http.Error(w, "Cabinet not configured", http.StatusServiceUnavailable)
		return
	}

	log.Printf("📥 Скачивание этикетки для заказа %s, task_id=%d", postingNumber, taskID)

	// Пытаемся получить этикетку с повторными попытками
	pdfData, err := services.GetLabelByTaskIDWithRetry(cabinet, taskID, 5, 2*time.Second)
	if err != nil {
		log.Printf("❌ Ошибка скачивания этикетки: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	// Сохраняем этикетку в файл
	filePath, err := services.SaveLabelToFile(cabinet, postingNumber, pdfData)
	if err != nil {
		log.Printf("❌ Ошибка сохранения этикетки: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	log.Printf("✅ Этикетка сохранена: %s", filePath)

	// Отдаем PDF клиенту
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.pdf", postingNumber))
	w.Write(pdfData)
}
