package services

import (
	"log"
	"sync/atomic"
	"time"
)

// Глобальный флаг для управления горутиной заказа этикеток
var KeyNeedLabels int32 = -1

// StartLabelWorker - запускает фоновую горутину для заказа этикеток
func StartLabelWorker() {
	go labelWorker()
	log.Println("🚀 Запущена горутина заказа этикеток")
}

// labelWorker - фоновая горутина, обрабатывает заказы с labels_status = 1
func labelWorker() {
	for {
		state := atomic.LoadInt32(&KeyNeedLabels)

		switch state {
		case -1:
			time.Sleep(5 * time.Second)
			continue

		case 0:
			log.Println("🔍 Ручной режим: проверка заказов на заказ этикеток")
			if err := processPendingLabels(); err != nil {
				log.Printf("⚠️ Ошибка при заказе этикеток: %v", err)
			}
			atomic.StoreInt32(&KeyNeedLabels, -1)
			log.Println("💤 Ручной режим завершен, ожидание новых задач")

		case 1:
			log.Println("🔄 Авто-режим: проверка заказов на заказ этикеток")
			if err := processPendingLabels(); err != nil {
				log.Printf("⚠️ Ошибка при заказе этикеток: %v", err)
			}
			time.Sleep(25 * time.Second)
			continue

		default:
			atomic.StoreInt32(&KeyNeedLabels, -1)
		}
	}
}

// WakeLabelWorker - пробуждает горутину для обработки заказов
func WakeLabelWorker() {
	if atomic.LoadInt32(&KeyNeedLabels) == -1 {
		atomic.StoreInt32(&KeyNeedLabels, 0)
		log.Println("🔔 Горутина этикеток пробуждена")
	}
}
