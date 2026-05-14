package services

import (
	"sync"
	"time"

	"ozon-api-separator/internal/models"
)

var (
	globalLabelQueue     *models.LabelQueue
	globalLabelQueueOnce sync.Once
)

func GetLabelQueue() *models.LabelQueue {
	globalLabelQueueOnce.Do(func() {
		globalLabelQueue = &models.LabelQueue{
			Jobs:        make(map[string][]string),
			Status:      make(map[string]string),
			Progress:    make(map[string]int),
			Total:       make(map[string]int),
			StartTime:   make(map[string]time.Time),
			Errors:      make(map[string]string),
			FailedItems: make(map[string][]string),
		}
	})
	return globalLabelQueue
}
