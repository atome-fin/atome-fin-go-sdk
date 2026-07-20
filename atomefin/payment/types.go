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
	OrderTypeTransport           PaymentOrderType = "TRANSPORT"
	OrderTypeGrabFood            PaymentOrderType = "GRAB_FOOD"
	OrderTypeGrabMart            PaymentOrderType = "GRAB_MART"
	OrderTypeSpecializedDelivery PaymentOrderType = "SPECIALIZED_DELIVERY"
)

// IsValid reports whether t is one of the spec-defined order types.
func (t PaymentOrderType) IsValid() bool {
	switch t {
	case OrderTypeTransport, OrderTypeGrabFood, OrderTypeGrabMart, OrderTypeSpecializedDelivery:
		return true
	}
	return false
}

// CreditProfile is an integration-agreed JSON risk-control payload
// (opaque string on the wire).
type CreditProfile string

// SubOrder is one line item inside an Auth / Capture request body.
//
// On /capture the SubOrders slice MUST equal the /auth set byte-for-byte
// (same items, same order, same amounts) — DESIGN.md §1.4.
type SubOrder struct {
	// SubOrderID is partner-generated and must be unique within a request.
	SubOrderID string `json:"subOrderId"`
	// Amount is the line-item charge in minor units.
	// Σ over SubOrders MUST equal AuthRequest.TotalAmount.
	Amount atomefin.Amount `json:"amount"`
	// Quantity is the number of units; >= 1.
	Quantity int `json:"quantity"`

	// PeriodType — installment tenor for this line; see Q17 for the
	// relationship to the order-level periodType.
	PeriodType *int `json:"periodType,omitempty"`
	// SkuID — product SKU identifier (required per spec).
	SkuID string `json:"skuId"`
	// SkuName — display label.
	SkuName string `json:"skuName,omitempty"`
	// SpuID — catalog parent identifier.
	SpuID string `json:"spuId,omitempty"`
	// CategoryID — partner taxonomy id (required per spec).
	CategoryID string `json:"categoryId"`
	// CategoryOneName — top-level category name (required per spec).
	CategoryOneName string `json:"categoryOneName"`
	// CategoryCodes — additional category codes; emit as `[]` not `null`
	// (R8: required-empty parity).
	CategoryCodes []string `json:"categoryCodes,omitempty"`
	// MerchantID — merchant or driver identifier (required per spec).
	MerchantID string `json:"merchantId"`
	// MerchantName — merchant or driver display name.
	MerchantName string `json:"merchantName,omitempty"`
	// MerchantCategory — merchant category (Food & Mart); omit for Transport.
	MerchantCategory string `json:"merchantCategory,omitempty"`
	// MerchantJoinedDate — merchant onboarding or driver registration date.
	MerchantJoinedDate string `json:"merchantJoinedDate,omitempty"`
	// ExtendInfo — per-line metadata bag (typed; no map[string]any).
	ExtendInfo *SubOrderExtendInfo `json:"extendInfo,omitempty"`
}

// SubOrderExtendInfo is the per-line metadata bag. The spec defines this
// as an open object; we model the known keys explicitly so the public
// surface is type-safe. Unknown keys round-trip
// opaquely via the Extras map only when the partner explicitly opts
// into it — for v0.1 we leave it empty and rely on forward-compat
// strict-decode failures to surface new fields.
type SubOrderExtendInfo struct {
	// Reserved for future spec additions. Today this struct is empty
	// because the spec defines SubOrder.extendInfo as a free object.
	// Keeping the type ensures we can add fields in a minor release
	// without breaking the AuthRequest signature.
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
type RequestExtendInfo struct {
	// UserCreditScore is the partner's risk score for the user,
	// 0..1. Modelled as *float64 so 0.0 (worst score) is
	// distinguishable from absent.
	//
	// This is the SOLE permitted public-surface float — a probability,
	// not money, so the int64-amount rule does not apply. Validate via
	// IsValidScore before passing to /auth.
	UserCreditScore *float64 `json:"userCreditScore,omitempty"`

	// OrderType is the Grab order business line (TRANSPORT, GRAB_FOOD, …).
	// Required by the spec on /auth and /capture.
	OrderType PaymentOrderType `json:"orderType"`

	// CreditProfile is an integration-agreed JSON risk-control payload.
	CreditProfile CreditProfile `json:"creditProfile,omitempty"`

	// DeviceInfo describes the device originating the request.
	DeviceInfo *DeviceInfo `json:"deviceInfo,omitempty"`

	// Address is the user's shipping address.
	// PII — DESIGN.md §10 redaction list.
	Address *Address `json:"address,omitempty"`

	// RiskInfo carries partner-side risk signals. Driver and trip
	// fields apply to TRANSPORT orders.
	RiskInfo *PaymentRiskInfo `json:"riskInfo,omitempty"`
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
type DeviceInfo struct {
	Platform  Platform       `json:"platform,omitempty"`
	GPS       *GeoPoint      `json:"gps"`
	Device    *DeviceProfile `json:"device,omitempty"`
	WifiList  []WifiAP       `json:"wifiList,omitempty"`
	IPAddress *IPAddress     `json:"ipAddress,omitempty"`
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
	GoogleAdvertisingID string     `json:"googleAdvertisingId,omitempty"`
	IDFA                string     `json:"idfa,omitempty"`
	IDFV                string     `json:"idfv,omitempty"`
	UTDID               string     `json:"utdid"`               // PII
	IsRoot              *bool      `json:"isRoot,omitempty"`    // pointer: distinguish false from absent
	AndroidID           string     `json:"androidId,omitempty"` // PII
	Build               *BuildInfo `json:"build,omitempty"`
}

// BuildInfo is the device's android.os.Build snapshot.
type BuildInfo struct {
	Board        string `json:"board,omitempty"`
	Brand        string `json:"brand,omitempty"`
	CPUABI       string `json:"cpuAbi,omitempty"`
	Device       string `json:"device,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	Product      string `json:"product,omitempty"`
}

// WifiAP is one Wi-Fi access point seen by the device.
// SSID is PII; emit `[]` not `null` when slice empty (R8).
type WifiAP struct {
	SSID string `json:"ssid"` // PII
}

// IPAddress carries the device's reported IPs.
// Strings are not parsed; partner responsibility to pass valid values.
type IPAddress struct {
	EthIP  string `json:"ethIp,omitempty"`
	TrueIP string `json:"trueIp,omitempty"`
}

// Address is the user's shipping address.
// Every field is PII per DESIGN.md §10.
type Address struct {
	ShippingAddress *ShippingAddress `json:"shippingAddress,omitempty"`
	ShippingName    string           `json:"shippingName,omitempty"`    // PII
	ShippingPhoneNo string           `json:"shippingPhoneNo,omitempty"` // PII
}

// ShippingAddress is the structured address sub-object.
type ShippingAddress struct {
	State    string `json:"state,omitempty"`
	City     string `json:"city,omitempty"`
	District string `json:"district,omitempty"`
	Address1 string `json:"address1,omitempty"`
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
	SubOrderID      string          `json:"subOrderId"`
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
	SubOrderID       string            `json:"subOrderId"`
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
