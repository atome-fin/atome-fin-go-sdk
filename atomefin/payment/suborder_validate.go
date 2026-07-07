package payment

import "github.com/atome-fin/atome-fin-go-sdk/atomefin"

// validatePlanSubOrder checks commerce-domain required fields on a
// PlanSubOrder (shared by /payment-precheck, /payment-plan, and aligned
// with /auth /capture SubOrder requirements).
func validatePlanSubOrder(so PlanSubOrder) error {
	if so.SubOrderID == "" {
		return &atomefin.ValidationError{Field: "subOrders[].subOrderId", Message: "required"}
	}
	if so.SkuID == "" {
		return &atomefin.ValidationError{Field: "subOrders[].skuId", Message: "required"}
	}
	if so.CategoryID == "" {
		return &atomefin.ValidationError{Field: "subOrders[].categoryId", Message: "required"}
	}
	if so.CategoryOneName == "" {
		return &atomefin.ValidationError{Field: "subOrders[].categoryOneName", Message: "required"}
	}
	if so.MerchantID == "" {
		return &atomefin.ValidationError{Field: "subOrders[].merchantId", Message: "required"}
	}
	if so.Amount <= 0 {
		return &atomefin.ValidationError{Field: "subOrders[].amount", Message: "must be > 0 (minor units)"}
	}
	if so.Quantity < 1 {
		return &atomefin.ValidationError{Field: "subOrders[].quantity", Message: "must be >= 1"}
	}
	return nil
}

// validateCommerceSubOrder checks commerce-domain required fields on a
// full SubOrder used by /auth and /capture.
func validateCommerceSubOrder(so SubOrder) error {
	return validatePlanSubOrder(PlanSubOrder{
		SubOrderID:      so.SubOrderID,
		SkuID:           so.SkuID,
		CategoryID:      so.CategoryID,
		CategoryOneName: so.CategoryOneName,
		MerchantID:      so.MerchantID,
		Amount:          so.Amount,
		Quantity:        so.Quantity,
	})
}
