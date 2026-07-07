package services

import (
	"log"
	"sync/atomic"
	"time"

	"ozon-api-separator/internal/config"
)

var KeyNeedLabels int32 = -1
var KeyDownloadLabels int32 = -1
var KeyAutoUpdateOrders int32 = 0

// StartLabelWorkers - запускает фоновые горутины
func StartLabelWorkers() {
	go labelWorker()
	go downloadWorker()
	go autoUpdateOrdersWorker()
	log.Println("[INFO] Запущены горутины заказа, скачивания этикеток и авто-обновления заказов")
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
			log.Println("[INFO] Ручной режим: проверка заказов на заказ этикеток")
			if err := processPendingLabels(); err != nil {
				log.Printf("[ERROR] Ошибка при заказе этикеток: %v", err)
			}
			atomic.StoreInt32(&KeyNeedLabels, -1)
			log.Println("[INFO] Ручной режим заказа завершен")

		case 1:
			if err := processPendingLabels(); err != nil {
				log.Printf("[ERROR] Ошибка при заказе этикеток: %v", err)
			}
			time.Sleep(25 * time.Second)
			continue

		default:
			atomic.StoreInt32(&KeyNeedLabels, -1)
		}
	}
}

// downloadWorker - фоновая горутина, обрабатывает заказы с labels_status = 2
func downloadWorker() {
	for {
		state := atomic.LoadInt32(&KeyDownloadLabels)

		switch state {
		case -1:
			time.Sleep(5 * time.Second)
			continue

		case 0:
			log.Println("[INFO] Ручной режим: проверка заказов на скачивание этикеток")
			time.Sleep(3 * time.Second)
			if err := processPendingDownloads(); err != nil {
				log.Printf("[ERROR] Ошибка при скачивании этикеток: %v", err)
			}
			atomic.StoreInt32(&KeyDownloadLabels, -1)
			log.Println("[INFO] Ручной режим скачивания завершен")

		case 1:
			time.Sleep(3 * time.Second)
			if err := processPendingDownloads(); err != nil {
				log.Printf("[ERROR] Ошибка при скачивании этикеток: %v", err)
			}
			time.Sleep(25 * time.Second)
			continue

		default:
			atomic.StoreInt32(&KeyDownloadLabels, -1)
		}
	}
}

// autoUpdateOrdersWorker - фоновая горутина для авто-обновления заказов
func autoUpdateOrdersWorker() {
	for {
		if atomic.LoadInt32(&KeyAutoUpdateOrders) == 1 {
			for key, cabinet := range config.AppConfig.Cabinets {
				if cabinet.ClientID == "" || cabinet.APIKey == "" {
					continue
				}

				if !config.IsAutoModeEnabledForCabinet(key) {
					continue
				}

				orders, err := GetAwaitingPackagingOrders(cabinet)
				if err != nil {
					log.Printf("[WARNING] Авто-обновление [%s]: ошибка загрузки заказов: %v", key, err)
					continue
				}

				if err := UpdateOrders(key, cabinet.Name, orders); err != nil {
					log.Printf("[WARNING] Авто-обновление [%s]: ошибка сохранения: %v", key, err)
					continue
				}

				log.Printf("[INFO] Авто-обновление [%s]: загружено %d заказов", key, len(orders))
			}

			if err := processReadyForSplitOrders(); err != nil {
				log.Printf("[WARNING] Авто-разделение: ошибка обработки заказов: %v", err)
			}

			time.Sleep(25 * time.Second)
		} else {
			time.Sleep(5 * time.Second)
		}
	}
}

// WakeLabelWorker - пробуждает горутину заказа этикеток
func WakeLabelWorker() {
	if atomic.LoadInt32(&KeyNeedLabels) == -1 {
		atomic.StoreInt32(&KeyNeedLabels, 0)
		log.Println("[INFO] Горутина заказа этикеток пробуждена")
	}
}

// WakeDownloadWorker - пробуждает горутину скачивания этикеток
func WakeDownloadWorker() {
	if atomic.LoadInt32(&KeyDownloadLabels) == -1 {
		atomic.StoreInt32(&KeyDownloadLabels, 0)
		log.Println("[INFO] Горутина скачивания этикеток пробуждена")
	}
}

// WakeAutoUpdateOrders - включает/выключает авто-обновление заказов
func WakeAutoUpdateOrders(enabled bool) {
	if enabled {
		atomic.StoreInt32(&KeyAutoUpdateOrders, 1)
		log.Println("[INFO] Авто-обновление заказов ВКЛЮЧЕНО")
	} else {
		atomic.StoreInt32(&KeyAutoUpdateOrders, 0)
		log.Println("[INFO] Авто-обновление заказов ВЫКЛЮЧЕНО")
	}
}
