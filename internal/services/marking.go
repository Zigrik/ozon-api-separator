package services

import (
	"fmt"
	"log"

	"ozon-api-separator/internal/models"
)

type Mark struct {
	Mark     string `json:"mark"`
	MarkType string `json:"mark_type"`
}
type Exemplar struct {
	ExemplarID   int64  `json:"exemplar_id"`
	IsGTDAbsent  bool   `json:"is_gtd_absent"`
	IsRNPTAbsent bool   `json:"is_rnpt_absent"`
	Marks        []Mark `json:"marks"`
}
type ProductExemplar struct {
	ProductID int64      `json:"product_id"`
	Exemplars []Exemplar `json:"exemplars"`
}
type markingSetRequest struct {
	PostingNumber string            `json:"posting_number"`
	Products      []ProductExemplar `json:"products"`
}

func AddMarkingsForOrder(cab *models.CabinetConfig, postingNumber string, productID int64, quantity int, codes []string) error {
	exemplars, err := GetExemplarIDs(cab, postingNumber)
	if err != nil {
		return err
	}
	var ids []int64
	for _, p := range exemplars.Products {
		if p.ProductID == productID {
			for _, e := range p.Exemplars {
				ids = append(ids, e.ExemplarID)
			}
			break
		}
	}
	if len(ids) < quantity {
		log.Printf("⚠️ недостаточно exemplar_id для товара %d", productID)
		return nil
	}

	marks := make([]Mark, quantity)
	for i := 0; i < quantity; i++ {
		marks[i] = Mark{Mark: codes[i], MarkType: "mandatory_mark"}
	}
	req := markingSetRequest{
		PostingNumber: postingNumber,
		Products: []ProductExemplar{
			{
				ProductID: productID,
				Exemplars: []Exemplar{
					{
						ExemplarID:   ids[0],
						IsGTDAbsent:  true,
						IsRNPTAbsent: true,
						Marks:        marks,
					},
				},
			},
		},
	}
	_, err = MakeOzonRequest(cab, "POST", "https://api-seller.ozon.ru/v6/fbs/posting/product/exemplar/set", req)
	return err
}

func SetGTDAsAbsent(cab *models.CabinetConfig, postingNumber string, productID int64) error {
	exemplars, err := GetExemplarIDs(cab, postingNumber)
	if err != nil {
		return fmt.Errorf("ошибка получения exemplar_id: %w", err)
	}

	var exemplarIDs []int64
	for _, p := range exemplars.Products {
		if p.ProductID == productID {
			for _, e := range p.Exemplars {
				exemplarIDs = append(exemplarIDs, e.ExemplarID)
			}
			break
		}
	}

	if len(exemplarIDs) == 0 {
		return fmt.Errorf("не найдены exemplar_id для товара %d в заказе %s", productID, postingNumber)
	}

	type GTDExemplar struct {
		ExemplarID   int64 `json:"exemplar_id"`
		IsGTDAbsent  bool  `json:"is_gtd_absent"`
		IsRNPTAbsent bool  `json:"is_rnpt_absent"`
	}

	type GTDProductExemplar struct {
		ProductID int64         `json:"product_id"`
		Exemplars []GTDExemplar `json:"exemplars"`
	}

	request := struct {
		PostingNumber string               `json:"posting_number"`
		Products      []GTDProductExemplar `json:"products"`
	}{
		PostingNumber: postingNumber,
		Products: []GTDProductExemplar{
			{
				ProductID: productID,
				Exemplars: make([]GTDExemplar, 0),
			},
		},
	}

	for _, id := range exemplarIDs {
		request.Products[0].Exemplars = append(request.Products[0].Exemplars, GTDExemplar{
			ExemplarID:   id,
			IsGTDAbsent:  true,
			IsRNPTAbsent: true,
		})
	}

	_, err = MakeOzonRequest(cab, "POST", "https://api-seller.ozon.ru/v6/fbs/posting/product/exemplar/set", request)
	return err
}
