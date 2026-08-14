package payment

import "github.com/atome-fin/atome-fin-go-sdk/atomefin"

const validOrderTypesMsg = "must be one of TRANSPORT | GRAB_FOOD | GRAB_MART"

// validatePlanSubOrders applies /payment-plan scenario rules:
// GRAB_MART requires merchantId + amount on every entry;
// GRAB_FOOD / TRANSPORT require exactly one entry with amount.
func validatePlanSubOrders(orderType PaymentOrderType, orders []PlanSubOrder) error {
	switch orderType {
	case OrderTypeGrabFood, OrderTypeTransport:
		if len(orders) != 1 {
			return &atomefin.ValidationError{
				Field:   "subOrders",
				Message: "must contain exactly one entry for " + string(orderType),
			}
		}
		if orders[0].Amount <= 0 {
			return &atomefin.ValidationError{Field: "subOrders[].amount", Message: "must be > 0 (minor units)"}
		}
	case OrderTypeGrabMart:
		if len(orders) == 0 {
			return &atomefin.ValidationError{Field: "subOrders", Message: "must be non-empty"}
		}
		for _, so := range orders {
			if so.MerchantID == "" {
				return &atomefin.ValidationError{Field: "subOrders[].merchantId", Message: "required for GRAB_MART"}
			}
			if so.Amount <= 0 {
				return &atomefin.ValidationError{Field: "subOrders[].amount", Message: "must be > 0 (minor units)"}
			}
		}
	default:
		if len(orders) == 0 {
			return &atomefin.ValidationError{Field: "subOrders", Message: "must be non-empty"}
		}
	}
	return nil
}

// validateAuthCaptureSubOrders applies /auth and /capture scenario
// rules from MerchantSubOrder overlays.
func validateAuthCaptureSubOrders(orderType PaymentOrderType, orders []SubOrder) error {
	switch orderType {
	case OrderTypeTransport:
		if len(orders) != 1 {
			return &atomefin.ValidationError{
				Field:   "subOrders",
				Message: "must contain exactly one entry for TRANSPORT",
			}
		}
		if orders[0].Amount <= 0 {
			return &atomefin.ValidationError{Field: "subOrders[].amount", Message: "must be > 0 (minor units)"}
		}
	case OrderTypeGrabFood:
		if len(orders) != 1 {
			return &atomefin.ValidationError{
				Field:   "subOrders",
				Message: "must contain exactly one entry for GRAB_FOOD",
			}
		}
		return validateFoodMartAuthSubOrder(orders[0])
	case OrderTypeGrabMart:
		if len(orders) == 0 {
			return &atomefin.ValidationError{Field: "subOrders", Message: "must be non-empty"}
		}
		for _, so := range orders {
			if err := validateFoodMartAuthSubOrder(so); err != nil {
				return err
			}
		}
	default:
		if len(orders) == 0 {
			return &atomefin.ValidationError{Field: "subOrders", Message: "must be non-empty"}
		}
	}
	return nil
}

func validateFoodMartAuthSubOrder(so SubOrder) error {
	if so.SubOrderID == "" {
		return &atomefin.ValidationError{Field: "subOrders[].subOrderId", Message: "required"}
	}
	if so.MerchantID == "" {
		return &atomefin.ValidationError{Field: "subOrders[].merchantId", Message: "required"}
	}
	if so.Amount <= 0 {
		return &atomefin.ValidationError{Field: "subOrders[].amount", Message: "must be > 0 (minor units)"}
	}
	return nil
}

// validatePlanMainOrderExtendInfos applies /payment-plan SKU rules:
// GRAB_MART requires merchantId + skuInfos (skuId, amount);
// GRAB_FOOD and TRANSPORT are not checked.
func validatePlanMainOrderExtendInfos(orderType PaymentOrderType, infos []MainOrderExtendInfo) error {
	if orderType == OrderTypeGrabMart {
		return validateRequiredSKUInfos(infos, "GRAB_MART")
	}
	return nil
}

// validateAuthCaptureMainOrderExtendInfos applies /auth and /capture
// SKU rules: GRAB_MART and GRAB_FOOD require merchantId + skuInfos
// (skuId, amount). TRANSPORT is not checked.
func validateAuthCaptureMainOrderExtendInfos(orderType PaymentOrderType, infos []MainOrderExtendInfo) error {
	switch orderType {
	case OrderTypeGrabMart, OrderTypeGrabFood:
		return validateRequiredSKUInfos(infos, string(orderType))
	}
	return nil
}

func validateRequiredSKUInfos(infos []MainOrderExtendInfo, scenario string) error {
	if len(infos) == 0 {
		return &atomefin.ValidationError{
			Field:   "extendInfo.mainOrderExtendInfos",
			Message: "required for " + scenario,
		}
	}
	for _, info := range infos {
		if info.MerchantID == "" {
			return &atomefin.ValidationError{
				Field:   "extendInfo.mainOrderExtendInfos[].merchantId",
				Message: "required",
			}
		}
		if len(info.SkuInfos) == 0 {
			return &atomefin.ValidationError{
				Field:   "extendInfo.mainOrderExtendInfos[].skuInfos",
				Message: "required (at least one SKU)",
			}
		}
		for _, sku := range info.SkuInfos {
			if sku.SkuID == "" {
				return &atomefin.ValidationError{
					Field:   "extendInfo.mainOrderExtendInfos[].skuInfos[].skuId",
					Message: "required",
				}
			}
			if sku.Amount <= 0 {
				return &atomefin.ValidationError{
					Field:   "extendInfo.mainOrderExtendInfos[].skuInfos[].amount",
					Message: "must be > 0 (minor units)",
				}
			}
		}
	}
	return nil
}

func sumPlanSubOrderAmount(orders []PlanSubOrder) atomefin.Amount {
	var sum atomefin.Amount
	for _, so := range orders {
		sum += so.Amount
	}
	return sum
}

func sumSubOrderAmount(orders []SubOrder) atomefin.Amount {
	var sum atomefin.Amount
	for _, so := range orders {
		sum += so.Amount
	}
	return sum
}
