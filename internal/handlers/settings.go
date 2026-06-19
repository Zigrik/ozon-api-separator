package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync/atomic"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/services"
)

var autoModeEnabled int32 = 0

func HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	loadingText := os.Getenv("LOADING_TEXT")
	if loadingText == "" {
		loadingText = "Трудолюбивые ослики делят и сортируют ваши заказы..."
	}
	customImage := ""
	if _, err := os.Stat("static/images/not_donkey.png"); err == nil {
		customImage = "not_donkey.png"
	} else if _, err := os.Stat("static/images/donkey.png"); err == nil {
		customImage = "donkey.png"
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"loading_text": loadingText,
		"custom_image": customImage,
	})
}

// HandleGlobalAutoMode - включает/выключает глобальный авто-режим
func HandleGlobalAutoMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Enabled {
		atomic.StoreInt32(&autoModeEnabled, 1)
		for key := range config.AppConfig.Cabinets {
			config.SetAutoModeForCabinet(key, true)
		}
		atomic.StoreInt32(&services.KeyNeedLabels, 1)
		atomic.StoreInt32(&services.KeyDownloadLabels, 1)
		// Включаем авто-обновление заказов
		services.WakeAutoUpdateOrders(true)
		log.Println("🤖 Глобальный авто-режим ВКЛЮЧЕН (включая авто-обновление заказов)")
	} else {
		atomic.StoreInt32(&autoModeEnabled, 0)
		for key := range config.AppConfig.Cabinets {
			config.SetAutoModeForCabinet(key, false)
		}
		atomic.StoreInt32(&services.KeyNeedLabels, -1)
		atomic.StoreInt32(&services.KeyDownloadLabels, -1)
		// Выключаем авто-обновление заказов
		services.WakeAutoUpdateOrders(false)
		log.Println("🤖 Глобальный авто-режим ВЫКЛЮЧЕН")
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"enabled": req.Enabled,
	})
}

// HandleGlobalAutoModeStatus - возвращает статус глобального авто-режима
func HandleGlobalAutoModeStatus(w http.ResponseWriter, r *http.Request) {
	enabled := atomic.LoadInt32(&autoModeEnabled) == 1
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"enabled": enabled,
	})
}

// HandleToggleAutoMode - заглушка
func HandleToggleAutoMode(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "error",
		"message": "Not implemented",
	})
}

// HandleGetAutoModeStatus - заглушка
func HandleGetAutoModeStatus(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"cabinets": make(map[string]bool),
	})
}
