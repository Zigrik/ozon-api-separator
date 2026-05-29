package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/models"
	"ozon-api-separator/internal/services"
)

func HandleGetFailedLabels(w http.ResponseWriter, r *http.Request) {
	cabinet := r.URL.Query().Get("cabinet")
	if cabinet == "" {
		http.Error(w, "cabinet required", http.StatusBadRequest)
		return
	}

	failed := models.LabelErrors.GetByCabinet(cabinet)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"count":  len(failed),
		"items":  failed,
	})
}

func HandleRetryFailedLabels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Cabinet string   `json:"cabinet"`
		Items   []string `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cabinet := config.AppConfig.Cabinets[req.Cabinet]
	if cabinet == nil {
		http.Error(w, "Cabinet not found", http.StatusNotFound)
		return
	}

	failed := models.LabelErrors.GetByCabinet(req.Cabinet)
	if len(failed) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "error",
			"message": "Нет ошибочных этикеток",
		})
		return
	}

	toRetry := make([]string, 0)
	if len(req.Items) == 0 {
		for _, f := range failed {
			toRetry = append(toRetry, f.PostingNumber)
		}
	} else {
		toRetry = req.Items
	}

	jobID := fmt.Sprintf("retry_%d", time.Now().UnixNano())
	queue := services.GetLabelQueue()
	queue.Lock()
	queue.Jobs[jobID] = toRetry
	queue.Status[jobID] = "pending"
	queue.Total[jobID] = len(toRetry)
	queue.Progress[jobID] = 0
	queue.StartTime[jobID] = time.Now()
	queue.Errors[jobID] = ""
	queue.FailedItems[jobID] = []string{}
	queue.Unlock()

	go services.ProcessLabelJob(jobID, queue)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"job_id": jobID,
		"count":  len(toRetry),
	})
}

func HandleClearFailedLabels(w http.ResponseWriter, r *http.Request) {
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

	models.LabelErrors.ClearByCabinet(req.Cabinet)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"message": "Ошибки очищены",
	})
}

func HandleDownloadFailedLabels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cabinet := r.URL.Query().Get("cabinet")
	if cabinet == "" {
		http.Error(w, "cabinet required", http.StatusBadRequest)
		return
	}

	failed := models.LabelErrors.GetByCabinet(cabinet)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"count":  len(failed),
		"items":  failed,
	})
}
