package handlers

import (
	"encoding/json"
	"net/http"
	"os"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

func HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	loadingText := os.Getenv("LOADING_TEXT")
	if loadingText == "" {
		loadingText = "Трудолюбивые ослики делят и сортируют ваши заказы..."
	}
	customImage := ""
	if _, err := os.Stat("static/images/not_donkey.png"); err == nil {
		customImage = "not_donkey.png"
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"loading_text": loadingText,
		"custom_image": customImage,
	})
}

func HandleToggleAutoMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Cabinet string `json:"cabinet"`
		Enabled bool   `json:"enabled"`
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
	config.SetAutoModeForCabinet(req.Cabinet, req.Enabled)
	if req.Enabled {
		services.StartCSVWorkerForCabinet(cabinet)
	} else {
		services.StopCSVWorkerForCabinet(req.Cabinet)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "enabled": req.Enabled})
}

func HandleGetAutoModeStatus(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"cabinets": config.GetAutoModeStatusForAllCabinets(),
	})
}
