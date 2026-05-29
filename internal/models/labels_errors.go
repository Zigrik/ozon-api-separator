package models

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type FailedLabel struct {
	PostingNumber string    `json:"posting_number"`
	CabinetKey    string    `json:"cabinet_key"`
	CabinetName   string    `json:"cabinet_name"`
	Error         string    `json:"error"`
	Timestamp     time.Time `json:"timestamp"`
	RetryCount    int       `json:"retry_count"`
}

type LabelErrorStore struct {
	sync.RWMutex
	FailedLabels map[string]*FailedLabel // key: posting_number
}

var LabelErrors = &LabelErrorStore{
	FailedLabels: make(map[string]*FailedLabel),
}

func (s *LabelErrorStore) Add(postingNumber, cabinetKey, cabinetName, errMsg string) {
	s.Lock()
	defer s.Unlock()

	if existing, exists := s.FailedLabels[postingNumber]; exists {
		existing.RetryCount++
		existing.Error = errMsg
		existing.Timestamp = time.Now()
	} else {
		s.FailedLabels[postingNumber] = &FailedLabel{
			PostingNumber: postingNumber,
			CabinetKey:    cabinetKey,
			CabinetName:   cabinetName,
			Error:         errMsg,
			Timestamp:     time.Now(),
			RetryCount:    1,
		}
	}

	s.saveToFile()
}

func (s *LabelErrorStore) Remove(postingNumber string) {
	s.Lock()
	defer s.Unlock()
	delete(s.FailedLabels, postingNumber)
	s.saveToFile()
}

func (s *LabelErrorStore) GetByCabinet(cabinetKey string) []*FailedLabel {
	s.RLock()
	defer s.RUnlock()

	result := make([]*FailedLabel, 0)
	for _, label := range s.FailedLabels {
		if label.CabinetKey == cabinetKey {
			result = append(result, label)
		}
	}
	return result
}

func (s *LabelErrorStore) GetAll() []*FailedLabel {
	s.RLock()
	defer s.RUnlock()

	result := make([]*FailedLabel, 0)
	for _, label := range s.FailedLabels {
		result = append(result, label)
	}
	return result
}

func (s *LabelErrorStore) ClearByCabinet(cabinetKey string) {
	s.Lock()
	defer s.Unlock()

	for key, label := range s.FailedLabels {
		if label.CabinetKey == cabinetKey {
			delete(s.FailedLabels, key)
		}
	}
	s.saveToFile()
}

func (s *LabelErrorStore) saveToFile() {
	data, err := json.MarshalIndent(s.FailedLabels, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile("label-errors.json", data, 0644)
}

func (s *LabelErrorStore) LoadFromFile() {
	s.Lock()
	defer s.Unlock()

	data, err := os.ReadFile("label-errors.json")
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.FailedLabels)
}
