package services

import (
	"fmt"
	"log"

	"ozon-api-separator/internal/models"
)

// ShipOrdersInternal - внутренняя функция для разделения заказов
// Используется как ручным режимом, так и авто-режимом
func ShipOrdersInternal(cabinet *models.CabinetConfig, ordersToShip []struct {
	PostingNumber string
	Products      []struct {
		ProductID int64
		OfferID   string
		Quantity  int
	}
}, wakeWorker bool) ([]map[string]interface{}, error) {
	results := make([]map[string]interface{}, 0)

	log.Printf("[DEBUG] ShipOrdersInternal: начало разделения %d заказов", len(ordersToShip))

	// Получаем актуальные заказы для исправления product_id
	orders, err := GetAwaitingPackagingOrders(cabinet, []int64{})
	if err != nil {
		log.Printf("[WARNING] Ошибка получения заказов для исправления product_id: %v", err)
	}
	ordersMap := make(map[string][]models.Product)
	for _, order := range orders {
		ordersMap[order.PostingNumber] = order.Products
	}

	for _, orderReq := range ordersToShip {
		result := map[string]interface{}{
			"posting_number": orderReq.PostingNumber,
		}

		log.Printf("[DEBUG] ShipOrdersInternal: обработка заказа %s, продуктов %d", orderReq.PostingNumber, len(orderReq.Products))

		packages := make([]models.ShipPackage, 0)
		productIDs := make([]int64, 0)

		for _, product := range orderReq.Products {
			productID := product.ProductID

			if productID == 0 {
				if products, exists := ordersMap[orderReq.PostingNumber]; exists {
					for _, p := range products {
						if p.OfferID == product.OfferID {
							productID = p.ProductID
							if productID == 0 {
								productID = p.SKU
							}
							break
						}
					}
					if productID == 0 && product.OfferID == "" {
						for _, p := range products {
							if p.SKU == product.ProductID {
								productID = p.ProductID
								if productID == 0 {
									productID = p.SKU
								}
								break
							}
						}
					}
				}
				if productID == 0 {
					log.Printf("[ERROR] Не удалось исправить ProductID=0 для заказа %s (offer_id=%s)", orderReq.PostingNumber, product.OfferID)
					result["status"] = "error"
					result["error"] = fmt.Sprintf("Не удалось определить товар для заказа %s", orderReq.PostingNumber)
					results = append(results, result)
					continue
				} else {
					log.Printf("[INFO] ProductID был 0, исправлен на %d для заказа %s (offer_id=%s)", productID, orderReq.PostingNumber, product.OfferID)
				}
			}

			productIDs = append(productIDs, productID)
			for i := 0; i < product.Quantity; i++ {
				packages = append(packages, models.ShipPackage{
					Products: []models.ShipProduct{
						{
							ProductID: productID,
							Quantity:  1,
						},
					},
				})
			}
		}

		if len(packages) == 0 {
			result["status"] = "error"
			result["error"] = "Нет товаров для отправки"
			results = append(results, result)
			continue
		}

		shipments, err := ShipOrder(cabinet, orderReq.PostingNumber, packages)
		if err != nil {
			log.Printf("[ERROR] Ошибка разделения заказа %s: %v", orderReq.PostingNumber, err)
			result["status"] = "error"
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}

		// Проверяем, что shipments не пустые
		if len(shipments) == 0 {
			log.Printf("[ERROR] ShipOrdersInternal: заказ %s разделен на 0 отправлений!", orderReq.PostingNumber)
			result["status"] = "error"
			result["error"] = "заказ разделен на 0 отправлений"
			results = append(results, result)
			continue
		}

		log.Printf("[INFO] Заказ %s разделён на %d отправлений", orderReq.PostingNumber, len(shipments))

		if err := UpdateOrderAfterShip(cabinet.Key, orderReq.PostingNumber, shipments, productIDs, 1); err != nil {
			log.Printf("[ERROR] Ошибка сохранения состояния после разделения: %v", err)
			result["status"] = "error"
			result["error"] = fmt.Sprintf("ошибка сохранения состояния: %v", err)
			results = append(results, result)
			continue
		}

		if wakeWorker {
			WakeLabelWorker()
			log.Printf("[INFO] Пробужден воркер заказа этикеток для заказа %s", orderReq.PostingNumber)
		}

		result["status"] = "success"
		result["shipments"] = shipments
		result["message"] = fmt.Sprintf("Заказ разделён на %d отправлений", len(shipments))

		results = append(results, result)
	}

	return results, nil
}
