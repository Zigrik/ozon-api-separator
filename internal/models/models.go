package models

import (
	"sync"
	"time"
)

type CabinetConfig struct {
	Name     string
	ClientID string
	APIKey   string
	Color    string
	BgColor  string
	Key      string
	DataPath string // общий путь для этикеток и CSV
}

type AppConfig struct {
	Password      string
	Cabinets      map[string]*CabinetConfig
	ActiveCabinet string
	AuthToken     string
}

type Posting struct {
	PostingNumber string         `json:"posting_number"`
	Status        string         `json:"status"`
	OrderID       int64          `json:"order_id"`
	CreatedAt     time.Time      `json:"created_at"`
	Products      []Product      `json:"products"`
	Requirements  *Requirements  `json:"requirements,omitempty"`
	AnalyticsData *AnalyticsData `json:"analytics_data,omitempty"`
	FinancialData *FinancialData `json:"financial_data,omitempty"`
	IsFolderReady bool           `json:"is_folder_ready"`
}

type Product struct {
	SKU                int64       `json:"sku"`
	Name               string      `json:"name"`
	Quantity           int         `json:"quantity"`
	ProductID          int64       `json:"product_id,omitempty"`
	OfferID            string      `json:"offer_id,omitempty"`
	Price              interface{} `json:"price,omitempty"`
	IsMandatoryMarked  bool        `json:"is_mandatory_marked"`
	IsGtdRequired      bool        `json:"is_gtd_required"`
	IsCountryRequired  bool        `json:"is_country_required"`
	IsMarkingCompleted bool        `json:"is_marking_completed"`
}

type AnalyticsData struct {
	City string `json:"city,omitempty"`
}

type FinancialData struct {
	Products []FinancialProduct `json:"products"`
}

type FinancialProduct struct {
	ProductID int64       `json:"product_id"`
	Price     interface{} `json:"price"`
	Quantity  int         `json:"quantity"`
}

type Requirements struct {
	ProductsRequiringGTD           []int64 `json:"products_requiring_gtd,omitempty"`
	ProductsRequiringCountry       []int64 `json:"products_requiring_country,omitempty"`
	ProductsRequiringMandatoryMark []int64 `json:"products_requiring_mandatory_mark,omitempty"`
	ProductsRequiringRNPT          []int64 `json:"products_requiring_rnpt,omitempty"`
	ProductsRequiringJWUIN         []int64 `json:"products_requiring_jw_uin,omitempty"`
	ProductsRequiringChangeCountry []int64 `json:"products_requiring_change_country,omitempty"`
	ProductsRequiringImei          []int64 `json:"products_requiring_imei,omitempty"`
	ProductsRequiringWeight        []int64 `json:"products_requiring_weight,omitempty"`
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
	AdditionalData []struct {
		PostingNumber string `json:"posting_number"`
		Products      []struct {
			CurrencyCode  string   `json:"currency_code"`
			MandatoryMark []string `json:"mandatory_mark"`
			Name          string   `json:"name"`
			OfferID       string   `json:"offer_id"`
			Price         string   `json:"price"`
			Quantity      int      `json:"quantity"`
			SKU           int64    `json:"sku"`
		} `json:"products"`
	} `json:"additional_data"`
	Result []string `json:"result"`
}

type CountryInfo struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type CountrySetRequest struct {
	PostingNumber  string `json:"posting_number"`
	ProductID      int64  `json:"product_id"`
	CountryISOCode string `json:"country_iso_code"`
}

type ExemplarCreateRequest struct {
	PostingNumber string `json:"posting_number"`
}

type ExemplarCreateResponse struct {
	PostingNumber string `json:"posting_number"`
	MultiBoxQty   int    `json:"multi_box_qty"`
	Products      []struct {
		ProductID               int64   `json:"product_id"`
		Quantity                int     `json:"quantity"`
		IsMandatoryMarkNeeded   bool    `json:"is_mandatory_mark_needed"`
		IsMandatoryMarkPossible bool    `json:"is_mandatory_mark_possible"`
		IsGTDNeeded             bool    `json:"is_gtd_needed"`
		IsRNPTNeeded            bool    `json:"is_rnpt_needed"`
		IsWeightNeeded          bool    `json:"is_weight_needed"`
		WeightMin               float64 `json:"weight_min"`
		WeightMax               float64 `json:"weight_max"`
		HasImei                 bool    `json:"has_imei"`
		IsJwUinNeeded           bool    `json:"is_jw_uin_needed"`
		Exemplars               []struct {
			ExemplarID   int64  `json:"exemplar_id"`
			GTD          string `json:"gtd"`
			IsGTDAbsent  bool   `json:"is_gtd_absent"`
			IsRNPTAbsent bool   `json:"is_rnpt_absent"`
			RNPT         string `json:"rnpt"`
			Weight       int    `json:"weight"`
			Marks        []struct {
				Mark     string `json:"mark"`
				MarkType string `json:"mark_type"`
			} `json:"marks"`
		} `json:"exemplars"`
	} `json:"products"`
}

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

type MarkForUpdate struct {
	ProductID int64
	Quantity  int
	Code      string
}

type ActionLog struct {
	Timestamp    string   `json:"timestamp"`
	OrderNumber  string   `json:"order_number"`
	Products     []string `json:"products"`
	SubOrders    []string `json:"sub_orders"`
	Marks        []string `json:"marks"`
	LabelSaved   bool     `json:"label_saved"`
	Success      bool     `json:"success"`
	ErrorMessage string   `json:"error_message,omitempty"`
}

type LabelQueue struct {
	Jobs        map[string][]string
	Status      map[string]string
	Progress    map[string]int
	Total       map[string]int
	StartTime   map[string]time.Time
	Errors      map[string]string
	FailedItems map[string][]string
	sync.Mutex
}
