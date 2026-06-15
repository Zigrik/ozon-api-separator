package models

import (
	"fmt"
	"strings"
	"time"
)

// CabinetConfig - конфигурация кабинета продавца Ozon
type CabinetConfig struct {
	Name     string // Название кабинета (например, "Шинорама")
	ClientID string // Client ID из личного кабинета Ozon
	APIKey   string // API Key из личного кабинета Ozon
	Key      string // Уникальный ключ кабинета (shinorama, trecktrack, sevenhundredshin)
}

// AppConfig - основная конфигурация приложения
type AppConfig struct {
	Password      string                    // Пароль для доступа к веб-интерфейсу
	Cabinets      map[string]*CabinetConfig // Словарь всех доступных кабинетов
	ActiveCabinet string                    // Ключ активного в данный момент кабинета
	AuthToken     string                    // Токен для автоматической авторизации (из .env)
}

// Posting - заказ (отгрузка) из Ozon API
type Posting struct {
	PostingNumber string        `json:"posting_number"`         // Номер отгрузки (заказа)
	Status        string        `json:"status"`                 // Статус заказа (awaiting_packaging и др.)
	OrderID       int64         `json:"order_id"`               // Внутренний ID заказа в Ozon
	CreatedAt     time.Time     `json:"created_at"`             // Дата создания
	Products      []Product     `json:"products"`               // Список товаров в заказе
	Requirements  *Requirements `json:"requirements,omitempty"` // Требования к товарам (маркировка, ГТД, страна)
	IsFolderReady bool          `json:"is_folder_ready"`        // Флаг наличия папки с этикетками (для фронтенда)
}

// Product - товар в составе заказа
type Product struct {
	SKU                int64       `json:"sku"`                            // Артикул Ozon
	Name               string      `json:"name"`                           // Название товара
	Quantity           int         `json:"quantity"`                       // Количество единиц
	ProductID          int64       `json:"product_id,omitempty"`           // ID товара в системе Ozon
	OfferID            string      `json:"offer_id,omitempty"`             // Артикул продавца
	Price              interface{} `json:"price,omitempty"`                // Цена за единицу (может быть строкой или числом)
	IsMandatoryMarked  bool        `json:"is_mandatory_marked,omitempty"`  // Требуется обязательная маркировка
	IsGtdRequired      bool        `json:"is_gtd_required,omitempty"`      // Требуется ГТД
	IsCountryRequired  bool        `json:"is_country_required,omitempty"`  // Требуется страна производителя
	IsMarkingCompleted bool        `json:"is_marking_completed,omitempty"` // Маркировка/ГТД уже добавлены
}

// Requirements - требования к товарам в заказе
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
