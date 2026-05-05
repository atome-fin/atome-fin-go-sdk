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
	// SkuID — mandate per merchant per Q13 (currently optional).
	SkuID string `json:"skuId,omitempty"`
	// CreatorID — merchant staff / channel that created this line.
	CreatorID string `json:"creatorId,omitempty"`
	// SkuName — display label.
	SkuName string `json:"skuName,omitempty"`
	// SpuID — catalog parent identifier.
	SpuID string `json:"spuId,omitempty"`
	// CategoryID — partner taxonomy id; max 128.
	CategoryID string `json:"categoryId,omitempty"` // max 128
	// CategoryOneName — top-level category name; max 128.
	CategoryOneName string `json:"categoryOneName,omitempty"` // max 128
	// CategoryCodes — additional category codes; emit as `[]` not `null`
	// (R8: required-empty parity).
	CategoryCodes []string `json:"categoryCodes,omitempty"`
	// OriginalAmount is the pre-discount amount in minor units.
	//
	// IMPORTANT: swagger.yaml lines 994/995 type this as `number`, but
	// per the user-confirmed money policy we
	// hold it as int64. Any partner-side fractional payload fails
	// decode loudly — caught by qa/marshal R11.
	OriginalAmount atomefin.Amount `json:"originalAmount,omitempty"`
	// MerchantID — partner-merchant identifier when the SubOrder
	// belongs to a sub-merchant.
	MerchantID string `json:"merchantId,omitempty"`
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

	// DeviceInfo describes the device originating the request.
	DeviceInfo *DeviceInfo `json:"deviceInfo,omitempty"`

	// Address is the user's shipping address.
	// PII — DESIGN.md §10 redaction list.
	Address *Address `json:"address,omitempty"`

	// RiskInfo carries partner-side risk signals. The spec body is
	// empty (Q12); kept as a typed placeholder until the partner
	// defines the shape.
	RiskInfo *PaymentRiskInfo `json:"riskInfo,omitempty"`

	// ReapplyTime is a Unix-ms timestamp; semantics per Q15 (open).
	// On the wire it is int64 ms-since-epoch — never time.Time.
	ReapplyTime *int64 `json:"reapplyTime,omitempty"`
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
	GPS       *GeoPoint      `json:"gps,omitempty"`
	Device    *DeviceProfile `json:"device,omitempty"`
	WifiList  []WifiAP       `json:"wifiList,omitempty"`
	IPAddress *IPAddress     `json:"ipAddress,omitempty"`
}

// GeoPoint is a (longitude, latitude, time) triple.
//
// The two coordinate floats here are NOT subject to the
// userCreditScore-only rule because they are geographical coordinates,
// not money. the spec forbids float for amounts; the rule does
// not apply to spatial / spec-level non-monetary numerics.
type GeoPoint struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
	Time      int64   `json:"time"` // Unix-ms
}

// DeviceProfile holds device fingerprint fields.
// Most fields are PII — DESIGN.md §10 redaction list.
type DeviceProfile struct {
	UTDID     string     `json:"utdid,omitempty"`     // PII
	IsRoot    *bool      `json:"isRoot,omitempty"`    // pointer: distinguish false from absent
	AndroidID string     `json:"androidId,omitempty"` // PII
	Build     *BuildInfo `json:"build,omitempty"`
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

// PaymentRiskInfo is reserved for the riskInfo block (Q12).
// The spec defines it as an empty object; we keep it typed so v0.2
// can fill it in without breaking the AuthRequest signature.
type PaymentRiskInfo struct{}

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
	InstallmentID string          `json:"installmentId"`
	DueDate       string          `json:"dueDate"` // yyyy-MM-dd; TZ unspecified (Q11)
	Amount        atomefin.Amount `json:"amount"`
	Principal     atomefin.Amount `json:"principal"`
	Fee           atomefin.Amount `json:"fee"`
	Interest      atomefin.Amount `json:"interest"`
}

// InstallmentPlan groups InstallmentDetail rows for a single SubOrder.
type InstallmentPlan struct {
	SubOrderID   string              `json:"subOrderId"`
	Installments []InstallmentDetail `json:"installments"`
}

// SubOrderInstallmentPlans is the response-side wrapper used in
// AuthorizationData.SubOrderInstallmentPlans and
// PaymentResult.SubOrderInstallmentPlans. The wire shape is identical
// to InstallmentPlan; we keep both types so DESIGN.md §5's response
// shape uses the spec's exact name.
type SubOrderInstallmentPlans = InstallmentPlan

// ---------- Response-side extendInfo ----------

// AuthExtendInfoResp is the AUTH-time response extendInfo. Carries a
// flat `agreementUrl` plus billing fields.
//
// **Distinct from CaptureExtendInfoResp** — see Q23. The two response
// shapes are not interchangeable; the auth-time URL and the
// capture-time URL are different artifacts.
type AuthExtendInfoResp struct {
	AgreementURL string `json:"agreementUrl,omitempty"` // PII per Q14
	Version      int64  `json:"version,omitempty"`      // Unix-ms (Q20)
	BillID       string `json:"billId,omitempty"`       // yyyyMM
	BillDate     string `json:"billDate,omitempty"`     // yyyy-MM-dd; TZ unspecified (Q11)
	DueDate      string `json:"dueDate,omitempty"`      // yyyy-MM-dd; TZ unspecified (Q11)
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
