package handlers

import (
	"encoding/json"
	"net/http"

	"ozon-api-separator/internal/config"
)

func HandleCheckPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Password == config.AppConfig.Password {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"token":  config.AppConfig.AuthToken,
		})
	} else {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid password"})
	}
}

func HandleSwitchCabinet(w http.ResponseWriter, r *http.Request) {
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
	_, exists := config.AppConfig.Cabinets[req.Cabinet]
	if !exists {
		http.Error(w, "Cabinet not found", http.StatusNotFound)
		return
	}
	config.AppConfig.ActiveCabinet = req.Cabinet
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "active": req.Cabinet})
}
