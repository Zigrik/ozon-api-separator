package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"ozon-api-separator/internal/config"
	"ozon-api-separator/internal/handlers"
	"ozon-api-separator/internal/middleware"
	"ozon-api-separator/internal/services"

	"github.com/Zigrik/license-system/license"
)

func getLocalIP() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "localhost"
	}
	addrs, err := net.LookupIP(hostname)
	if err == nil {
		for _, addr := range addrs {
			if ipv4 := addr.To4(); ipv4 != nil && !addr.IsLoopback() && ipv4[0] != 169 {
				return ipv4.String()
			}
		}
	}
	return "localhost"
}

func runApp() {
	if err := config.LoadConfig(); err != nil {
		log.Fatalf("Ошибка конфигурации: %v", err)
	}
	if err := config.LoadMarkingCodes(); err != nil {
		log.Printf("Ошибка кодов маркировки: %v", err)
	}

	if sec := os.Getenv("MONITOR_INTERVAL_SEC"); sec != "" {
		if s, err := strconv.Atoi(sec); err == nil && s > 0 {
			config.MonitorInterval = time.Duration(s) * time.Second
		}
	}
	config.LoadAutoModeSettings()
	services.StartAllAutoWorkers()

	os.MkdirAll("templates", 0755)
	os.MkdirAll("static", 0755)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "templates/index.html")
	})
	http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, r.URL.Path[1:])
	})
	http.HandleFunc("/api/check-password", handlers.HandleCheckPassword)
	http.HandleFunc("/api/settings", handlers.HandleGetSettings)
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.ico")
	})

	http.HandleFunc("/api/cabinet/switch", middleware.AuthMiddleware(handlers.HandleSwitchCabinet))
	http.HandleFunc("/api/orders", middleware.AuthMiddleware(handlers.HandleGetOrders))
	http.HandleFunc("/api/orders/ship", middleware.AuthMiddleware(handlers.HandleShipOrders))
	http.HandleFunc("/api/codes/available", middleware.AuthMiddleware(handlers.HandleGetAvailableCodes))
	http.HandleFunc("/api/markings/add-with-gtd", middleware.AuthMiddleware(handlers.HandleAddMarkingsWithGTD))
	http.HandleFunc("/api/countries/list", middleware.AuthMiddleware(handlers.HandleGetCountries))
	http.HandleFunc("/api/countries/set", middleware.AuthMiddleware(handlers.HandleSetCountry))
	http.HandleFunc("/api/labels/generate", middleware.AuthMiddleware(handlers.HandleStartLabelGenerationForShipments))
	http.HandleFunc("/api/labels/status", middleware.AuthMiddleware(handlers.HandleGetLabelStatus))
	http.HandleFunc("/api/labels/cancel", middleware.AuthMiddleware(handlers.HandleCancelLabelGeneration))
	http.HandleFunc("/api/labels/retry", middleware.AuthMiddleware(handlers.HandleRetryLabelGeneration))
	http.HandleFunc("/api/auto-mode/toggle", middleware.AuthMiddleware(handlers.HandleToggleAutoMode))
	http.HandleFunc("/api/auto-mode/status", middleware.AuthMiddleware(handlers.HandleGetAutoModeStatus))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		port = "8080"
	}

	log.Printf("Сервер запущен на http://%s:%s (http://localhost:%s)", getLocalIP(), port, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func main() {
	result := license.CheckLicense(
		"vGlxAZOhrJY+VjopJqaSQAc4e8zW9qAj2G5coWmQ3X4=",
		"license.key",
		"OZON Api Cabinet",
	)
	if !result.Valid {
		log.Fatalf("Ошибка лицензии: %s", result.Error)
	}
	log.Printf("✅ Лицензия активна. Компания: %s", result.Company)
	runApp()
}
