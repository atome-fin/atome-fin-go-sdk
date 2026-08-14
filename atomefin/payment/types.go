package payment

import "github.com/atome-fin/atome-fin-go-sdk/atomefin"

// This file hosts the shared sub-types used by the AuthRequest /
// CaptureRequest / *Response shapes. Per the spec:
//
//   - No interface{} or map[string]any anywhere on the public surface.
//   - Money fields are bare int64; required money fields never use
//     ,omitempty (so legitimate zero-deltas serialise).
//   - userCreditScore is the SOLE permitted public-surface float
//     (it is a probability, not money — see RequestExtendInfo).
//   - JSON tags are byte-for-byte from the spec (lowerCamelCase).
//   - Time fields stay int64 ms-since-epoch on the wire — no time.Time
//     in the JSON struct.
//
// Sub-types are grouped by concern: SubOrder + SubOrderExtendInfo;
// the request-side extendInfo deep tree (RequestExtendInfo through
// ShippingAddress); AccountChanges; the installment plan tree; the
// response-side extendInfo shapes (separate AuthExtendInfoResp and
// CaptureExtendInfoResp per Q23).

// ---------- SubOrder ----------

// PaymentOrderType is the order business line (Grab integration).
type PaymentOrderType string

const (
	OrderTypeTransport PaymentOrderType = "TRANSPORT"
	OrderTypeGrabFood  PaymentOrderType = "GRAB_FOOD"
	OrderTypeGrabMart  PaymentOrderType = "GRAB_MART"
)

// IsValid reports whether t is one of the spec-defined order types.
func (t PaymentOrderType) IsValid() bool {
	switch t {
	case OrderTypeTransport, OrderTypeGrabFood, OrderTypeGrabMart:
		return true
	}
	return false
}

// CreditProfile is an integration-agreed JSON risk-control payload
// (opaque string on the wire).
type CreditProfile string

// SubOrder is one merchant-dimension entry inside an Auth / Capture
// request body. Per the spec's MerchantSubOrder schema (and the
// FullMerchantSubOrder / FoodAuthMerchantSubOrder overlays):
//
//   - GRAB_MART auth/capture: SubOrderID + MerchantID + Amount
//     required; MerchantName / MerchantCategory required at the wire
//     level for Mart; we keep them non-omitempty so an empty value
//     still serialises (the server validates scenario rules).
//   - GRAB_FOOD auth/capture: SubOrderID + MerchantID + Amount.
//   - TRANSPORT: Amount only; exactly one entry.
//
// On /capture the SubOrders slice MUST equal the /auth set
// byte-for-byte (same items, same order, same amounts) — DESIGN.md §1.4.
//
// SKU-level detail lives under
// RequestExtendInfo.MainOrderExtendInfos[].SkuInfos, NOT here.
type SubOrder struct {
	// SubOrderID is partner-generated and must be unique within a request.
	// Required for GRAB_MART / GRAB_FOOD on /auth and /capture.
	SubOrderID string `json:"subOrderId,omitempty"`
	// MerchantID links this sub-order to a merchant (or driver for
	// legacy transport integrations). Required for GRAB_MART /
	// GRAB_FOOD auth/capture.
	MerchantID string `json:"merchantId,omitempty"`
	// MerchantName is the merchant display name (Food & Mart).
	MerchantName string `json:"merchantName,omitempty"`
	// MerchantCategory is the merchant category (Food & Mart).
	MerchantCategory string `json:"merchantCategory,omitempty"`
	// MerchantJoinedDate is the merchant onboarding date; optional in
	// all scenarios per spec.
	MerchantJoinedDate string `json:"merchantJoinedDate,omitempty"`
	// Amount is the sub-order (merchant) total in minor units.
	Amount atomefin.Amount `json:"amount"`
	// PeriodType is the installment tenor for this sub-order when
	// applicable (e.g. 1, 3, 6, 9, 12).
	PeriodType *int `json:"periodType,omitempty"`
}

// SkuInfo is one SKU line item under
// MainOrderExtendInfo.SkuInfos (spec's SkuInfo /
// MartAuthCaptureSkuInfo / FoodAuthSkuInfo). Amount is required in
// every scenario that sends SkuInfos; SkuID is required for
// GRAB_MART (plan/auth/capture) and GRAB_FOOD (auth/capture).
type SkuInfo struct {
	SkuID           string          `json:"skuId,omitempty"`
	SkuName         string          `json:"skuName,omitempty"`
	CategoryID      string          `json:"categoryId,omitempty"`
	CategoryOneName string          `json:"categoryOneName,omitempty"`
	CategoryCodes   []string        `json:"categoryCodes,omitempty"`
	Quantity        int             `json:"quantity,omitempty"`
	Amount          atomefin.Amount `json:"amount"`
}

// MainOrderExtendInfo is the per-merchant extension block under
// extendInfo.mainOrderExtendInfos[]. Spec's MainOrderExtendInfo
// requires merchantId + skuInfos when the array is present
// (GRAB_MART plan/auth/capture, GRAB_FOOD auth/capture);
// FoodPlanMainOrderExtendInfo relaxes both to optional.
type MainOrderExtendInfo struct {
	MerchantID string    `json:"merchantId,omitempty"`
	SkuInfos   []SkuInfo `json:"skuInfos,omitempty"`
}

// ---------- Request-side extendInfo deep tree ----------

// Platform is `extendInfo.deviceInfo.platform`. Closed enum per spec.
type Platform string

// Platform values defined by the spec.
const (
	PlatformAndroid Platform = "ANDROID"
	PlatformIOS     Platform = "IOS"
)

// IsValid reports whether p is one of the spec-defined platforms.
// Forward-compat: unknown values round-trip opaquely so a future spec
// addition does not break decoding (callers can still compare strings).
func (p Platform) IsValid() bool {
	return p == PlatformAndroid || p == PlatformIOS
}

// RequestExtendInfo is the top of the request-side extendInfo tree.
// Per the spec's AuthCaptureExtendInfoBase overlays, /auth and
// /capture require: orderType, creditProfile, deviceInfo, address.
// GRAB_MART / GRAB_FOOD additionally require mainOrderExtendInfos.
type RequestExtendInfo struct {
	// UserCreditScore is optional and legacy — not part of the
	// white-label G spec's extendInfo trees. Retained for internal
	// risk pipelines; omit when unused.
	UserCreditScore *float64 `json:"userCreditScore,omitempty"`

	// OrderType is the Grab order business line (TRANSPORT, GRAB_FOOD, …).
	// Required by the spec on /auth and /capture.
	OrderType PaymentOrderType `json:"orderType"`

	// CreditProfile is an integration-agreed JSON risk-control payload.
	// Required by the spec on /auth and /capture.
	CreditProfile CreditProfile `json:"creditProfile"`

	// DeviceInfo describes the device originating the request.
	// Required by the spec on /auth and /capture.
	DeviceInfo *DeviceInfo `json:"deviceInfo"`

	// Address is the user's shipping address. Required on /auth and
	// /capture. PII — DESIGN.md §10 redaction list.
	Address *Address `json:"address"`

	// RiskInfo carries partner-side risk signals. Driver and trip
	// fields apply to TRANSPORT orders.
	RiskInfo *PaymentRiskInfo `json:"riskInfo,omitempty"`

	// MainOrderExtendInfos carries SKU detail per merchant.
	// Required for GRAB_MART (plan/auth/capture) and GRAB_FOOD
	// (auth/capture); omit for TRANSPORT.
	MainOrderExtendInfos []MainOrderExtendInfo `json:"mainOrderExtendInfos,omitempty"`
}

// IsValidScore reports whether s is in the spec's 0..1 range.
// Returns true for nil (absent) so callers can validate without
// repeating the nil-check.
func IsValidScore(s *float64) bool {
	if s == nil {
		return true
	}
	return *s >= 0 && *s <= 1
}

// DeviceInfo describes the originating device.
// Required by the spec on /auth and /capture.
type DeviceInfo struct {
	Platform  Platform       `json:"platform"`
	GPS       *GeoPoint      `json:"gps,omitempty"`
	Device    *DeviceProfile `json:"device"`
	WifiList  []WifiAP       `json:"wifiList"`
	IPAddress *IPAddress     `json:"ipAddress"`
}

// GeoPoint is a (longitude, latitude, time) triple. The spec models
// all three values as strings in PlatformDeviceInfo.
type GeoPoint struct {
	Longitude string `json:"longitude"`
	Latitude  string `json:"latitude"`
	Time      string `json:"time"` // Unix-ms as a string
}

// DeviceProfile holds device fingerprint fields.
// Most fields are PII — DESIGN.md §10 redaction list.
type DeviceProfile struct {
	DeviceID            string     `json:"deviceId"`
	GoogleAdvertisingID string     `json:"googleAdvertisingId"`
	IDFA                string     `json:"idfa,omitempty"`
	IDFV                string     `json:"idfv,omitempty"`
	UTDID               string     `json:"utdid,omitempty"` // PII; optional per spec
	IsRoot              *bool      `json:"isRoot"`
	AndroidID           string     `json:"androidId,omitempty"` // PII
	Build               *BuildInfo `json:"build"`
}

// BuildInfo is the device's android.os.Build snapshot.
type BuildInfo struct {
	Board        string `json:"board"`
	Brand        string `json:"brand"`
	CPUABI       string `json:"cpuAbi,omitempty"`
	Device       string `json:"device"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Product      string `json:"product"`
}

// WifiAP is one Wi-Fi access point seen by the device.
// SSID is PII; emit `[]` not `null` when slice empty (R8).
type WifiAP struct {
	SSID string `json:"ssid"` // PII
}

// IPAddress carries the device's reported IPs.
// Strings are not parsed; partner responsibility to pass valid values.
type IPAddress struct {
	EthIP  string `json:"ethIp"`
	TrueIP string `json:"trueIp"`
}

// Address is the user's shipping address.
// Every field is PII per DESIGN.md §10.
// Spec-required on /auth and /capture.
type Address struct {
	ShippingAddress *ShippingAddress `json:"shippingAddress"`
	ShippingName    string           `json:"shippingName"`    // PII
	ShippingPhoneNo string           `json:"shippingPhoneNo"` // PII
}

// ShippingAddress is the structured address sub-object.
type ShippingAddress struct {
	State    string `json:"state,omitempty"`
	City     string `json:"city"`
	District string `json:"district,omitempty"`
	Address1 string `json:"address1"`
	Address2 string `json:"address2,omitempty"`
}

// PaymentRiskInfo is the auth/capture riskInfo block.
type PaymentRiskInfo struct {
	DriverInfo   *DriverInfo   `json:"driverInfo,omitempty"`
	TripMetadata *TripMetadata `json:"tripMetadata,omitempty"`
}

// DriverInfo carries transport driver signals.
type DriverInfo struct {
	AverageRating *float64 `json:"averageRating,omitempty"`
	IsVerified    *bool    `json:"isVerified,omitempty"`
}

// TripMetadata carries transport trip context.
type TripMetadata struct {
	EstimatedDistance float64 `json:"estimatedDistance,omitempty"`
	PickupLocation    string  `json:"pickupLocation,omitempty"`
}

// ---------- AccountChanges ----------

// AccountChanges is the credit-change vector populated when an
// auth/capture/void mutates the user's account state.5
// pins the canonical 11-field shape.
//
// All *Change fields are SIGNED int64 deltas — negatives must round-trip
// (a refund event reduces UsedCredit; the test corpus exercises at
// least one negative delta per R10). Required money fields use bare
// `json:"name"` (no ,omitempty) so a legitimate zero delta serialises.
//
// PreviousStatus is **position-scoped**: it accepts
// only NORMAL / ACCOUNT_BLOCKED_OVERDUE / ACCOUNT_BLOCKED.
// `ACCOUNT_CLOSED` is valid for CurrentStatus only. Use
// IsValidPreviousStatus / IsValidCurrentStatus to enforce this at the
// call site; unknown values still round-trip opaquely per the
// forward-compat rule.
type AccountChanges struct {
	Version              int64                  `json:"version"` // Unix-ms
	ExternalReferenceUID string                 `json:"externalReferenceUid"`
	PreviousStatus       atomefin.AccountStatus `json:"previousStatus"`
	CurrentStatus        atomefin.AccountStatus `json:"currentStatus"`

	TotalCreditChange     atomefin.Amount `json:"totalCreditChange"`
	UsedCreditChange      atomefin.Amount `json:"usedCreditChange"`
	FrozenCreditChange    atomefin.Amount `json:"frozenCreditChange"`
	AvailableCreditChange atomefin.Amount `json:"availableCreditChange"`
	OverpaidAmountChange  atomefin.Amount `json:"overpaidAmountChange"`
	LateFeeAmountChange   atomefin.Amount `json:"lateFeeAmountChange"`
	InterestAmountChange  atomefin.Amount `json:"interestAmountChange"`
}

// IsValidPreviousStatus reports whether s is a valid AccountChanges
// previousStatus. ACCOUNT_CLOSED is REJECTED here (a closed account
// cannot have been the prior state for an account-mutation event).
//
// Distinct from "unknown enum" semantics: this returns false for
// ACCOUNT_CLOSED specifically; for forward-compat, partners that see
// an unknown literal in production should escalate it to QA rather
// than silently accept it. Use this function at validation time, not
// at decode time.
func IsValidPreviousStatus(s atomefin.AccountStatus) bool {
	switch s {
	case atomefin.AccountStatusNormal,
		atomefin.AccountStatusBlockedOverdue,
		atomefin.AccountStatusBlocked:
		return true
	}
	return false
}

// IsValidCurrentStatus reports whether s is a valid AccountChanges
// currentStatus. Unlike PreviousStatus, this DOES accept
// ACCOUNT_CLOSED.
func IsValidCurrentStatus(s atomefin.AccountStatus) bool {
	switch s {
	case atomefin.AccountStatusNormal,
		atomefin.AccountStatusBlockedOverdue,
		atomefin.AccountStatusBlocked,
		atomefin.AccountStatusClosed:
		return true
	}
	return false
}

// ---------- Installment plan tree ----------

// InstallmentDetail is one row of a per-sub-order installment schedule.
// All money fields are minor units (R10/R12 corpus applies).
type InstallmentDetail struct {
	SubOrderID      string          `json:"subOrderId,omitempty"`
	TotalTenor      int             `json:"totalTenor"`
	CurrentTenor    int             `json:"currentTenor"`
	RepayAmount     atomefin.Amount `json:"repayAmount"`
	PrincipalAmount atomefin.Amount `json:"principalAmount"`
	InterestAmount  atomefin.Amount `json:"interestAmount"`
	BillID          string          `json:"billId"`
	BillDate        string          `json:"billDate"`
	DueDate         string          `json:"dueDate"`
}

// InstallmentPlan is one tenor option for a sub-order.
type InstallmentPlan struct {
	TotalTenor          int                 `json:"totalTenor"`
	OrderRepayAmount    atomefin.Amount     `json:"orderRepayAmount"`
	OrderInterestAmount atomefin.Amount     `json:"orderInterestAmount"`
	InstallmentDetails  []InstallmentDetail `json:"installmentDetails"`
}

// SubOrderInstallmentPlans is the response-side wrapper used in
// AuthorizationData.SubOrderInstallmentPlans and
// PaymentResult.SubOrderInstallmentPlans.
type SubOrderInstallmentPlans struct {
	SubOrderID string `json:"subOrderId,omitempty"`
	// MerchantID echoes the request merchant for GRAB_MART / GRAB_FOOD
	// per MartAuthCaptureSubOrderInstallmentPlans /
	// FoodAuthCaptureSubOrderInstallmentPlans. Optional for TRANSPORT.
	MerchantID       string            `json:"merchantId,omitempty"`
	OrderAmount      atomefin.Amount   `json:"orderAmount"`
	InstallmentPlans []InstallmentPlan `json:"installmentPlans"`
}

// ---------- Response-side extendInfo ----------

// AuthExtendInfoResp is the AUTH-time response extendInfo.
type AuthExtendInfoResp struct {
	ReapplyTime *int64 `json:"reapplyTime,omitempty"` // Unix-ms
}

// CaptureExtendInfoResp is the CAPTURE-time response extendInfo
// . Carries a NESTED `agreement.agreementUrl` plus
// `reapplyTime`.
//
// **Distinct from AuthExtendInfoResp** — see Q23.
type CaptureExtendInfoResp struct {
	ReapplyTime *int64        `json:"reapplyTime,omitempty"` // Unix-ms (Q15)
	Agreement   *AgreementRef `json:"agreement,omitempty"`
}

// AgreementRef is the nested agreement object inside
// CaptureExtendInfoResp.
type AgreementRef struct {
	// AgreementURL is required when the parent Agreement is emitted.
	AgreementURL string `json:"agreementUrl"` // PII per Q14
}
