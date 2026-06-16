package models

import (
	"fmt"
	"strings"
	"time"
)

// ============ КОНФИГУРАЦИЯ ============

// CabinetConfig - конфигурация кабинета продавца Ozon
type CabinetConfig struct {
	Name     string // Название кабинета (например, "Шинорама")
	ClientID string // Client ID из личного кабинета Ozon
	APIKey   string // API Key из личного кабинета Ozon
	Key      string // Уникальный ключ кабинета (shinorama, trecktrack, sevenhundredshin)
	DataPath string // Путь для сохранения этикеток
}

// AppConfig - основная конфигурация приложения
type AppConfig struct {
	Password      string                    // Пароль для доступа к веб-интерфейсу
	Cabinets      map[string]*CabinetConfig // Словарь всех доступных кабинетов
	ActiveCabinet string                    // Ключ активного в данный момент кабинета
	AuthToken     string                    // Токен для автоматической авторизации
}

// ============ МОДЕЛИ OZON API ============

// Posting - заказ (отгрузка) из Ozon API
type Posting struct {
	PostingNumber string        `json:"posting_number"`
	Status        string        `json:"status"`
	OrderID       int64         `json:"order_id"`
	CreatedAt     time.Time     `json:"created_at"`
	Products      []Product     `json:"products"`
	Requirements  *Requirements `json:"requirements,omitempty"`
	IsFolderReady bool          `json:"is_folder_ready"`
}

// Product - товар в составе заказа
type Product struct {
	SKU                int64       `json:"sku"`
	Name               string      `json:"name"`
	Quantity           int         `json:"quantity"`
	ProductID          int64       `json:"product_id,omitempty"`
	OfferID            string      `json:"offer_id,omitempty"`
	Price              interface{} `json:"price,omitempty"`
	IsMandatoryMarked  bool        `json:"is_mandatory_marked,omitempty"`
	IsGtdRequired      bool        `json:"is_gtd_required,omitempty"`
	IsCountryRequired  bool        `json:"is_country_required,omitempty"`
	IsMarkingCompleted bool        `json:"is_marking_completed,omitempty"`
}

// Requirements - требования к товарам в заказе (из Ozon API)
type Requirements struct {
	ProductsRequiringGTD           []int64 `json:"products_requiring_gtd,omitempty"`
	ProductsRequiringCountry       []int64 `json:"products_requiring_country,omitempty"`
	ProductsRequiringMandatoryMark []int64 `json:"products_requiring_mandatory_mark,omitempty"`
}

// GetPrice - возвращает цену как float64
func (p *Product) GetPrice() float64 {
	switch v := p.Price.(type) {
	case float64:
		return v
	case string:
		var price float64
		cleanStr := strings.ReplaceAll(v, ",", "")
		cleanStr = strings.ReplaceAll(cleanStr, " ", "")
		fmt.Sscanf(cleanStr, "%f", &price)
		return price
	default:
		return 0
	}
}

// PostingsListResponse - ответ Ozon API на запрос списка отгрузок
type PostingsListResponse struct {
	Result struct {
		Postings []Posting `json:"postings"`
	} `json:"result"`
}

// PostingsFilter - фильтр для запроса списка отгрузок
type PostingsFilter struct {
	Filter struct {
		Status     string     `json:"status,omitempty"`
		CutoffFrom *time.Time `json:"cutoff_from,omitempty"`
		CutoffTo   *time.Time `json:"cutoff_to,omitempty"`
	} `json:"filter"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// ============ МОДЕЛИ ДЛЯ РАЗДЕЛЕНИЯ ЗАКАЗОВ ============

// ShipRequest - запрос на разделение заказа
type ShipRequest struct {
	PostingNumber string        `json:"posting_number"`
	Packages      []ShipPackage `json:"packages"`
}

// ShipPackage - упаковка (отдельное отправление)
type ShipPackage struct {
	Products []ShipProduct `json:"products"`
}

// ShipProduct - товар в упаковке
type ShipProduct struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

// ShipResponse - ответ на разделение заказа
type ShipResponse struct {
	Result []string `json:"result"`
}

// ============ МОДЕЛИ ДЛЯ ЭТИКЕТОК ============

// CreateLabelRequest - запрос на создание задачи для этикеток
type CreateLabelRequest struct {
	PostingNumbers []string `json:"posting_number"`
}

// CreateLabelResponse - ответ на создание задачи для этикеток
type CreateLabelResponse struct {
	Result struct {
		Tasks []struct {
			TaskID   int64  `json:"task_id"`
			TaskType string `json:"task_type"`
		} `json:"tasks"`
	} `json:"result"`
}

// GetLabelRequest - запрос на получение этикетки по ID задачи
type GetLabelRequest struct {
	TaskID int64 `json:"task_id"`
}

// GetLabelResponse - ответ на получение этикетки
type GetLabelResponse struct {
	Result struct {
		Error   string `json:"error"`
		Status  string `json:"status"`
		FileURL string `json:"file_url"`
	} `json:"result"`
}

// ============ МОДЕЛИ ДЛЯ СОСТОЯНИЯ ЗАКАЗОВ ============

// CabinetState - состояние всех заказов в кабинете
type CabinetState struct {
	CabinetKey  string       `json:"cabinet_key"`
	CabinetName string       `json:"cabinet_name"`
	LastUpdated time.Time    `json:"last_updated"`
	Orders      []OrderState `json:"orders"`
}

// OrderState - состояние одного заказа
type OrderState struct {
	PostingNumber string          `json:"posting_number"`
	IsDivided     bool            `json:"is_divided"`
	Products      []ProductState  `json:"products"`
	Shipments     []ShipmentState `json:"shipments"`
	LabelsStatus  int             `json:"labels_status"` // 0-3
	Errors        []OrderError    `json:"errors"`
}

// ProductState - состояние товара в заказе
type ProductState struct {
	ProductID    int64              `json:"product_id"`
	OfferID      string             `json:"offer_id"`
	Quantity     int                `json:"quantity"`
	Requirements ProductRequirement `json:"requirements"`
	Marking      MarkingState       `json:"marking"`
	Country      CountryState       `json:"country"`
}

// ProductRequirement - требования к товару (для состояния)
type ProductRequirement struct {
	IsMandatoryMarked bool `json:"is_mandatory_marked"`
	IsGtdRequired     bool `json:"is_gtd_required"`
	IsCountryRequired bool `json:"is_country_required"`
}

// MarkingState - состояние маркировки
type MarkingState struct {
	IsCompleted bool     `json:"is_completed"`
	Codes       []string `json:"codes"`
	GtdAbsent   bool     `json:"gtd_absent"`
	Error       *string  `json:"error"`
}

// CountryState - состояние страны производителя
type CountryState struct {
	IsCompleted bool    `json:"is_completed"`
	Code        *string `json:"code"`
	Error       *string `json:"error"`
}

// ShipmentState - состояние подзаказа (отправления)
type ShipmentState struct {
	PostingNumber string     `json:"posting_number"`
	ProductIDs    []int64    `json:"product_ids"`
	Label         LabelState `json:"label"`
}

// LabelState - состояние этикетки
type LabelState struct {
	TaskID       int64   `json:"task_id"`
	IsOrdered    bool    `json:"is_ordered"`
	IsDownloaded bool    `json:"is_downloaded"`
	FilePath     string  `json:"file_path"`
	Error        *string `json:"error"`
}

// OrderError - ошибка по заказу
type OrderError struct {
	PostingNumber string `json:"posting_number"`
	Operation     string `json:"operation"`
	Error         string `json:"error"`
	Timestamp     string `json:"timestamp"`
}

// ============ МЕТОДЫ ДЛЯ РАБОТЫ С СОСТОЯНИЕМ ============

// GetFolderName - возвращает имя папки для сохранения этикеток
func (o *OrderState) GetFolderName() string {
	parts := strings.Split(o.PostingNumber, "-")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return o.PostingNumber
}

// IsReadyForShipment - готов ли товар к отправке
func (p *ProductState) IsReadyForShipment() bool {
	if !p.Requirements.IsMandatoryMarked &&
		!p.Requirements.IsGtdRequired &&
		!p.Requirements.IsCountryRequired {
		return true
	}

	if p.Requirements.IsMandatoryMarked || p.Requirements.IsGtdRequired {
		if !p.Marking.IsCompleted {
			return false
		}
	}

	if p.Requirements.IsCountryRequired {
		if !p.Country.IsCompleted {
			return false
		}
	}

	return true
}

// GetReadyProducts - возвращает товары готовые к отправке
func (o *OrderState) GetReadyProducts() []ProductState {
	result := make([]ProductState, 0)
	for _, p := range o.Products {
		if p.IsReadyForShipment() {
			result = append(result, p)
		}
	}
	return result
}

// HasReadyProducts - есть ли товары готовые к отправке
func (o *OrderState) HasReadyProducts() bool {
	return len(o.GetReadyProducts()) > 0
}

// CalculateLabelsStatus - вычисляет статус этикеток
func (o *OrderState) CalculateLabelsStatus() int {
	if len(o.Shipments) == 0 {
		return 0
	}

	allOrdered := true
	allDownloaded := true

	for _, s := range o.Shipments {
		if !s.Label.IsOrdered {
			allOrdered = false
		}
		if !s.Label.IsDownloaded {
			allDownloaded = false
		}
	}

	if allDownloaded {
		return 3
	}
	if allOrdered {
		return 2
	}
	return 1
}
