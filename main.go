package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

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
		log.Fatalf("❌ Ошибка конфигурации: %v", err)
	}

	os.MkdirAll("templates", 0755)
	os.MkdirAll("static", 0755)
	os.MkdirAll("orders", 0755)
	os.MkdirAll("data", 0755)

	services.StartLabelWorkers()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "templates/index.html")
	})
	http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, r.URL.Path[1:])
	})
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.ico")
	})

	// API без авторизации
	http.HandleFunc("/api/check-password", handlers.HandleCheckPassword)

	// API с авторизацией
	http.HandleFunc("/api/cabinet/switch", middleware.AuthMiddleware(handlers.HandleSwitchCabinet))
	http.HandleFunc("/api/orders", middleware.AuthMiddleware(handlers.HandleGetOrders))
	http.HandleFunc("/api/orders/ship", middleware.AuthMiddleware(handlers.HandleShipOrders))
	http.HandleFunc("/api/orders/ship-and-labels", middleware.AuthMiddleware(handlers.HandleShipAndOrderLabels))
	http.HandleFunc("/api/orders/state", middleware.AuthMiddleware(handlers.HandleGetOrderState))
	http.HandleFunc("/api/orders/stats", middleware.AuthMiddleware(handlers.HandleGetStats))

	// API для страны, ГТД и маркировки
	http.HandleFunc("/api/countries/list", middleware.AuthMiddleware(handlers.HandleGetCountries))
	http.HandleFunc("/api/countries/set", middleware.AuthMiddleware(handlers.HandleSetCountry))
	http.HandleFunc("/api/gtd/absent", middleware.AuthMiddleware(handlers.HandleSetGTDAbsent))
	http.HandleFunc("/api/markings/add", middleware.AuthMiddleware(handlers.HandleAddMarkings))

	// API для этикеток
	http.HandleFunc("/api/labels/create", middleware.AuthMiddleware(handlers.HandleCreateLabels))
	http.HandleFunc("/api/labels/status", middleware.AuthMiddleware(handlers.HandleGetLabelStatus))
	http.HandleFunc("/api/labels/download", middleware.AuthMiddleware(handlers.HandleDownloadLabel))
	http.HandleFunc("/api/labels/trigger-order", middleware.AuthMiddleware(handlers.HandleTriggerOrderLabels))
	http.HandleFunc("/api/labels/trigger-download", middleware.AuthMiddleware(handlers.HandleTriggerDownloadLabels))

	// API для авто-режима
	http.HandleFunc("/api/auto-mode/global", middleware.AuthMiddleware(handlers.HandleGlobalAutoMode))
	http.HandleFunc("/api/auto-mode/global-status", middleware.AuthMiddleware(handlers.HandleGlobalAutoModeStatus))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if _, err := strconv.Atoi(port); err != nil {
		port = "8080"
	}

	localIP := getLocalIP()
	log.Printf("🚀 Сервер запущен на http://%s:%s (http://localhost:%s)", localIP, port, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func main() {
	result := license.CheckLicense(
		"vGlxAZOhrJY+VjopJqaSQAc4e8zW9qAj2G5coWmQ3X4=",
		"license.key",
		"OZON Api Cabinet",
	)

	if !result.Valid {
		log.Fatalf("❌ Ошибка лицензии: %s", result.Error)
	}

	log.Printf("✅ Лицензия активна. Компания: %s", result.Company)
	runApp()
}
