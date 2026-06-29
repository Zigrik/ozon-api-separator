package models

import (
	"fmt"
	"strings"
	"time"
)

// ============ КОНФИГУРАЦИЯ ============

type CabinetConfig struct {
	Name     string
	ClientID string
	APIKey   string
	Key      string
	DataPath string
	Color    string
	BgColor  string
}

type AppConfig struct {
	Password      string
	Cabinets      map[string]*CabinetConfig
	ActiveCabinet string
	AuthToken     string
}

// ============ МОДЕЛИ OZON API ============

type Posting struct {
	PostingNumber   string        `json:"posting_number"`
	Status          string        `json:"status"`
	OrderID         int64         `json:"order_id"`
	CreatedAt       time.Time     `json:"created_at"`
	Products        []Product     `json:"products"`
	Requirements    *Requirements `json:"requirements,omitempty"`
	IsFolderReady   bool          `json:"is_folder_ready"`
	IsReadyForSplit bool          `json:"is_ready_for_split,omitempty"`
}

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

type Requirements struct {
	ProductsRequiringGTD           []int64 `json:"products_requiring_gtd,omitempty"`
	ProductsRequiringCountry       []int64 `json:"products_requiring_country,omitempty"`
	ProductsRequiringMandatoryMark []int64 `json:"products_requiring_mandatory_mark,omitempty"`
}

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

type PostingsListResponse struct {
	Result struct {
		Postings []Posting `json:"postings"`
	} `json:"result"`
}

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

type ShipRequest struct {
	PostingNumber string        `json:"posting_number"`
	Packages      []ShipPackage `json:"packages"`
}

type ShipPackage struct {
	Products []ShipProduct `json:"products"`
}

type ShipProduct struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

type ShipResponse struct {
	Result []string `json:"result"`
}

// ============ МОДЕЛИ ДЛЯ ЭТИКЕТОК ============

type CreateLabelRequest struct {
	PostingNumbers []string `json:"posting_number"`
}

type CreateLabelResponse struct {
	Result struct {
		Tasks []struct {
			TaskID   int64  `json:"task_id"`
			TaskType string `json:"task_type"`
		} `json:"tasks"`
	} `json:"result"`
}

type GetLabelRequest struct {
	TaskID int64 `json:"task_id"`
}

type GetLabelResponse struct {
	Result struct {
		Error   string `json:"error"`
		Status  string `json:"status"`
		FileURL string `json:"file_url"`
	} `json:"result"`
}

// ============ МОДЕЛИ ДЛЯ СОСТОЯНИЯ ЗАКАЗОВ ============

type CabinetState struct {
	CabinetKey  string       `json:"cabinet_key"`
	CabinetName string       `json:"cabinet_name"`
	LastUpdated time.Time    `json:"last_updated"`
	Orders      []OrderState `json:"orders"`
}

type OrderState struct {
	PostingNumber   string          `json:"posting_number"`
	IsReadyForSplit bool            `json:"is_ready_for_split"`
	IsDivided       bool            `json:"is_divided"`
	Products        []ProductState  `json:"products"`
	Shipments       []ShipmentState `json:"shipments"`
	LabelsStatus    int             `json:"labels_status"`
	Errors          []OrderError    `json:"errors"`
}

type ProductState struct {
	ProductID    int64              `json:"product_id"`
	SKU          int64              `json:"sku"`
	OfferID      string             `json:"offer_id"`
	Quantity     int                `json:"quantity"`
	Requirements ProductRequirement `json:"requirements"`
	Marking      MarkingState       `json:"marking"`
	Country      CountryState       `json:"country"`
}

type ProductRequirement struct {
	IsMandatoryMarked bool `json:"is_mandatory_marked"`
	IsGtdRequired     bool `json:"is_gtd_required"`
	IsCountryRequired bool `json:"is_country_required"`
}

type MarkingState struct {
	IsCompleted bool     `json:"is_completed"`
	Codes       []string `json:"codes"`
	GtdAbsent   bool     `json:"gtd_absent"`
	Error       *string  `json:"error"`
}

type CountryState struct {
	IsCompleted bool    `json:"is_completed"`
	Code        *string `json:"code"`
	Error       *string `json:"error"`
}

type ShipmentState struct {
	PostingNumber string     `json:"posting_number"`
	ProductIDs    []int64    `json:"product_ids"`
	Label         LabelState `json:"label"`
}

type LabelState struct {
	TaskID       int64   `json:"task_id"`
	IsOrdered    bool    `json:"is_ordered"`
	IsDownloaded bool    `json:"is_downloaded"`
	FilePath     string  `json:"file_path"`
	RetryCount   int     `json:"retry_count"`
	Error        *string `json:"error"`
}

type OrderError struct {
	PostingNumber string `json:"posting_number"`
	Operation     string `json:"operation"`
	Error         string `json:"error"`
	Timestamp     string `json:"timestamp"`
}

// ============ МОДЕЛИ ДЛЯ СТРАНЫ, ГТД И МАРКИРОВКИ ============

type CountryInfo struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type SetCountryRequest struct {
	PostingNumber  string `json:"posting_number"`
	ProductID      int64  `json:"product_id"`
	CountryISOCode string `json:"country_iso_code"`
}

type ExemplarCreateRequest struct {
	PostingNumber string `json:"posting_number"`
}

type ExemplarCreateResponse struct {
	PostingNumber string `json:"posting_number"`
	Products      []struct {
		ProductID int64 `json:"product_id"`
		Exemplars []struct {
			ExemplarID int64 `json:"exemplar_id"`
		} `json:"exemplars"`
	} `json:"products"`
}

// ============ МОДЕЛИ ДЛЯ СТАТУСА МАРКИРОВКИ ============

// ExemplarStatusRequest - запрос на получение статуса маркировки
type ExemplarStatusRequest struct {
	PostingNumber string `json:"posting_number"`
}

// ExemplarStatusResponse - ответ на запрос статуса маркировки
type ExemplarStatusResponse struct {
	PostingNumber string `json:"posting_number"`
	Status        string `json:"status"`
	Products      []struct {
		ProductID int64 `json:"product_id"`
		Exemplars []struct {
			ExemplarID   int64  `json:"exemplar_id"`
			GTD          string `json:"gtd"`
			IsGTDAbsent  bool   `json:"is_gtd_absent"`
			IsRNPTAbsent bool   `json:"is_rnpt_absent"`
			RNPT         string `json:"rnpt"`
			Weight       int    `json:"weight"`
			Marks        []struct {
				Mark        string   `json:"mark"`
				MarkType    string   `json:"mark_type"`
				CheckStatus string   `json:"check_status"`
				ErrorCodes  []string `json:"error_codes"`
			} `json:"marks"`
			GTDCheckStatus    string   `json:"gtd_check_status"`
			GTDErrorCodes     []string `json:"gtd_error_codes"`
			RNPTCheckStatus   string   `json:"rnpt_check_status"`
			RNPTErrorCodes    []string `json:"rnpt_error_codes"`
			WeightCheckStatus string   `json:"weight_check_status"`
			WeightErrorCodes  []string `json:"weight_error_codes"`
		} `json:"exemplars"`
	} `json:"products"`
}

type Mark struct {
	Mark     string `json:"mark"`
	MarkType string `json:"mark_type"`
}

type MarkingSetRequest struct {
	PostingNumber string `json:"posting_number"`
	Products      []struct {
		ProductID int64 `json:"product_id"`
		Exemplars []struct {
			ExemplarID   int64  `json:"exemplar_id"`
			IsGTDAbsent  bool   `json:"is_gtd_absent"`
			IsRNPTAbsent bool   `json:"is_rnpt_absent"`
			Marks        []Mark `json:"marks"`
		} `json:"exemplars"`
	} `json:"products"`
}

// ============ МЕТОДЫ ДЛЯ РАБОТЫ С СОСТОЯНИЕМ ============

func (o *OrderState) GetFolderName() string {
	parts := strings.Split(o.PostingNumber, "-")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return o.PostingNumber
}

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

func (o *OrderState) GetReadyProducts() []ProductState {
	result := make([]ProductState, 0)
	for _, p := range o.Products {
		if p.IsReadyForShipment() {
			result = append(result, p)
		}
	}
	return result
}

func (o *OrderState) HasReadyProducts() bool {
	return len(o.GetReadyProducts()) > 0
}

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
