package credit

import "github.com/atome-fin/atome-fin-go-sdk/atomefin"

// This file hosts every typed struct on the credit sub-package's
// public surface. The naming convention mirrors refund / bill /
// transaction:
//
//   - Request bodies are *Param (mirrors refund.RefundParam).
//   - Response envelopes are *Response (Code/Message/Data spine).
//   - Inner data structures keep the spec's name verbatim where
//     possible (CreditApplicationResult, CreditInformationResult,
//     CreditApplicationCollectQueryResult).
//
// All money fields are atomefin.Amount (int64; minor units). The
// Currency type is atomefin.Currency (named string; only "IDR"
// supported per Q10 RESOLVED 2026-05-06). Time fields are int64
// Unix-ms per project policy (no time.Time on JSON structs).

// ---------- Spec-additional Code constants (credit-domain) ----------
//
// Code values that appear ONLY on credit-domain envelopes — kept in
// this package rather than atomefin/codes.go so the umbrella package
// doesn't grow per-domain enum spam. Partners that want a single
// errors.As(*atomefin.APIError) site can still match on the typed
// Code value because all constants here are atomefin.Code-typed.

const (
	// CodeActiveAccount — returned by POST /credit-information and
	// POST /close-account when the user already has an active account
	// on Atome.
	CodeActiveAccount atomefin.Code = "ACTIVE_ACCOUNT"
	// CodeCreditApplicationInProgress — returned by POST
	// /credit-information when an application is already being
	// processed for this user. Spec spelling preserved
	// ("INPROGESS", not "INPROGRESS"); do NOT correct it — the wire
	// literal is partner-facing.
	CodeCreditApplicationInProgress atomefin.Code = "CREDIT_APPLICATION_INPROGESS"
	// CodeAccountClosed — surfaces when the user's account has been
	// closed (typically on /query-balance-history).
	CodeAccountClosed atomefin.Code = "ACCOUNT_CLOSED"
	// CodeUnpaidDebt / CodeOverpaidUnreturned / CodeOngoingWithdrawal /
	// CodeUserAccountNotExist / CodeFailed — POST /close-account
	// outcome variants per spec (lines 729-768).
	CodeUnpaidDebt          atomefin.Code = "UNPAID_DEBT"
	CodeOverpaidUnreturned  atomefin.Code = "OVERPAID_UNRETURNED"
	CodeOngoingWithdrawal   atomefin.Code = "ONGOING_WITHDRAWAL"
	CodeUserAccountNotExist atomefin.Code = "USER_ACCOUNT_NOT_EXIST"
	CodeFailed              atomefin.Code = "FAILED"
)

// ---------- Status enums (credit-domain) ----------

// CreditStatus is the lifecycle state of a credit application or
// information submission. Closed enum per the spec convention;
// unknown values round-trip opaquely (forward-compat).
type CreditStatus string

// Spec-defined credit statuses.
const (
	// CreditStatusSuccess — application approved / information
	// submitted successfully.
	CreditStatusSuccess CreditStatus = "SUCCESS"
	// CreditStatusFailed — application rejected / information accept
	// failed.
	CreditStatusFailed CreditStatus = "FAILED"
	// CreditStatusProcessing — application is under examination.
	// Returned by /credit-result when async examination is still
	// running.
	CreditStatusProcessing CreditStatus = "PROCESSING"
	// CreditStatusDraft — credit information submission has been
	// received, but the credit application has not yet been
	// submitted. Returned by /credit-information and
	// /credit-information-result.
	CreditStatusDraft CreditStatus = "DRAFT"
)

// IsValid reports whether s is a spec-defined credit status.
// Forward-compat: unknown values round-trip opaquely.
func (s CreditStatus) IsValid() bool {
	switch s {
	case CreditStatusSuccess, CreditStatusFailed, CreditStatusProcessing, CreditStatusDraft:
		return true
	}
	return false
}

// IsTerminal reports whether the status is SUCCESS or FAILED.
func (s CreditStatus) IsTerminal() bool {
	return s == CreditStatusSuccess || s == CreditStatusFailed
}

// String returns the wire literal verbatim.
func (s CreditStatus) String() string { return string(s) }

// Country is the ISO-style market code on credit-application
// payloads. Currently only "ID" (Indonesia) is supported.
type Country string

// Spec-defined countries.
const (
	// CountryIndonesia — Indonesia.
	CountryIndonesia Country = "ID"
)

// IsValid reports whether c is a spec-defined country.
func (c Country) IsValid() bool {
	return c == CountryIndonesia
}

// String returns the wire literal verbatim.
func (c Country) String() string { return string(c) }

// Language is the partner-facing display language for the
// Atome-hosted information collection page. The wire enum is closed
// at {en, id}.
type Language string

// Spec-defined languages.
const (
	LanguageEnglish    Language = "en"
	LanguageIndonesian Language = "id"
)

// IsValid reports whether l is a spec-defined language.
func (l Language) IsValid() bool {
	switch l {
	case LanguageEnglish, LanguageIndonesian:
		return true
	}
	return false
}

// BalanceHistoryType discriminates the kind of credit-balance change
// queried by /query-balance-history.
type BalanceHistoryType string

// Spec-defined balance-history types.
const (
	BalanceHistoryTypeOverpaidChange             BalanceHistoryType = "OVERPAID_CHANGE"
	BalanceHistoryTypeCreditLimitAdjustment      BalanceHistoryType = "CREDIT_LIMIT_ADJUSTMENT"
	BalanceHistoryTypeTradeAvailableCreditChange BalanceHistoryType = "TRADE_AVAILABLE_CREDIT_CHANGE"
)

// IsValid reports whether t is a spec-defined balance-history type.
func (t BalanceHistoryType) IsValid() bool {
	switch t {
	case BalanceHistoryTypeOverpaidChange,
		BalanceHistoryTypeCreditLimitAdjustment,
		BalanceHistoryTypeTradeAvailableCreditChange:
		return true
	}
	return false
}

// String returns the wire literal verbatim.
func (t BalanceHistoryType) String() string { return string(t) }

// LoanStatus is the loan-side lifecycle state surfaced on
// LoanCreditInfo. Closed enum at {NORMAL, RISK_REJECTED}.
type LoanStatus string

// Spec-defined loan statuses.
const (
	LoanStatusNormal       LoanStatus = "NORMAL"
	LoanStatusRiskRejected LoanStatus = "RISK_REJECTED"
)

// IsValid reports whether s is a spec-defined loan status.
func (s LoanStatus) IsValid() bool {
	switch s {
	case LoanStatusNormal, LoanStatusRiskRejected:
		return true
	}
	return false
}

// UserStatus is the user-account lifecycle state surfaced on
// CreditInfo and AccountChangeCreditInfo.
type UserStatus string

// Spec-defined user statuses.
const (
	UserStatusNormal                UserStatus = "NORMAL"
	UserStatusAccountBlockedOverdue UserStatus = "ACCOUNT_BLOCKED_OVERDUE"
	UserStatusAccountBlocked        UserStatus = "ACCOUNT_BLOCKED"
	UserStatusAccountClosed         UserStatus = "ACCOUNT_CLOSED"
)

// IsValid reports whether s is a spec-defined user status.
func (s UserStatus) IsValid() bool {
	switch s {
	case UserStatusNormal, UserStatusAccountBlockedOverdue,
		UserStatusAccountBlocked, UserStatusAccountClosed:
		return true
	}
	return false
}

// String returns the wire literal verbatim.
func (s UserStatus) String() string { return string(s) }

// FailReason is one entry of the CreditApplicationResult.failReason
// list — the rejection reason for a FAILED application.
type FailReason string

// Spec-defined failure reasons (lines 4039-4045 of swagger.yaml).
const (
	FailReasonFraud               FailReason = "FRAUD"
	FailReasonTooManyRetry        FailReason = "TOO_MANY_RETRY"
	FailReasonBlurredIDPhoto      FailReason = "BLURRED_ID_PHOTO"
	FailReasonIDInvalid           FailReason = "ID_INVALID"
	FailReasonNameIDNumberIsWrong FailReason = "NAME_ID_NUMBER_IS_WRONG"
	FailReasonWrongPhotoFormat    FailReason = "WRONG_PHOTO_FORMAT"
	FailReasonOthers              FailReason = "OTHERS"
)

// ---------- Request body types ----------

// CreditInformationParam is the POST /credit-information request
// body. Lightweight first step of the credit flow: the partner
// announces user identity and country; the server returns a jumpUrl
// into Atome's KYC web flow plus a requestId the partner echoes on
// the subsequent POST /credit-application.
type CreditInformationParam struct {
	// RequestID is partner-generated; max 64 chars. Idempotency key.
	RequestID string `json:"requestId"` // max 64
	// ExternalReferenceUID is the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"` // max 64
	// MobileNumber is the user's mobile with dialling code prefix,
	// e.g. "+62XXXXXXXXXXX". Required.
	MobileNumber string `json:"mobileNumber"`
	// Email is the user's email; required, max 64 chars.
	Email string `json:"email"` // max 64
	// Country is the ISO-style market code; currently locked to "ID".
	Country Country `json:"country"`
	// ApplicationEssentialInfo carries the minimal KYC fields for
	// this step (today individualProfile.ocrResult.fullName).
	ApplicationEssentialInfo *CreditInformationEssentialInfo `json:"applicationEssentialInfo"`
	// ExtendInfo carries extension fields agreed per integration —
	// today the only key is `language` for the KYC page (optional;
	// defaults to `id` when omitted).
	ExtendInfo *CreditInformationExtendInfo `json:"extendInfo,omitempty"`
}

// CreditInformationEssentialInfo is the applicationEssentialInfo bag
// on /credit-information. It intentionally uses credit-information
// specific nested types so this lightweight step cannot accidentally
// send the broader /credit-application KYC payload.
type CreditInformationEssentialInfo struct {
	IndividualProfile *CreditInformationIndividualProfile `json:"individualProfile"`
}

// CreditInformationIndividualProfile is the individualProfile bag on
// /credit-information. Swagger currently allows only ocrResult.fullName
// at this step.
type CreditInformationIndividualProfile struct {
	OCRResult *CreditInformationOCRResult `json:"ocrResult"`
}

// CreditInformationOCRResult is the OCR subset allowed on
// /credit-information.
type CreditInformationOCRResult struct {
	FullName string `json:"fullName"`
}

// CreditInformationExtendInfo is the extendInfo bag on a
// /credit-information request.
type CreditInformationExtendInfo struct {
	// Language is the display language for the Atome-hosted
	// information collection page. Optional; defaults to `id`.
	Language Language `json:"language,omitempty"`
}

// CreditApplicationParam is the POST /credit-application request
// body. Full KYC payload. Must reference a prior successful
// /credit-information's requestId via
// ExtendInfo.CreditInformationRequestID.
type CreditApplicationParam struct {
	// RequestID is partner-generated; max 64 chars. Idempotency key.
	RequestID string `json:"requestId"`
	// ExternalReferenceUID is the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// MobileNumber is the user's mobile with dialling code, required.
	MobileNumber string `json:"mobileNumber"`
	// Email is the user's email; required, max 64 chars.
	Email string `json:"email"`
	// Country is the ISO-style market code; currently "ID" only.
	Country Country `json:"country"`
	// ApplicationEssentialInfo carries the KYC blob (idType, OCR
	// fields, residential, work, platform, others). Required.
	ApplicationEssentialInfo *ApplicationEssentialInfo `json:"applicationEssentialInfo"`
	// ExtendInfo links to the prior /credit-information call.
	// Required (per spec).
	ExtendInfo *CreditApplicationExtendInfo `json:"extendInfo"`
}

// CreditApplicationExtendInfo is the extendInfo bag on a
// /credit-application request.
type CreditApplicationExtendInfo struct {
	// CreditInformationRequestID is the requestId from the prior
	// successful POST /credit-information for the same user; used to
	// correlate the two-step flow. Required, max 64 chars.
	CreditInformationRequestID string `json:"creditInformationRequestId"` // max 64
}

// CreditApplicationChangeParam is the POST /modify-application-info
// request body. Can only be requested when the credit application is
// approved (per spec).
type CreditApplicationChangeParam struct {
	// RequestID is partner-generated; max 64 chars.
	RequestID string `json:"requestId"`
	// ExternalReferenceUID is the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// MobileNumber is the new mobile with dialling code prefix;
	// required.
	MobileNumber string `json:"mobileNumber"`
	// Email is an optional updated email address.
	Email string `json:"email,omitempty"`
	// ExtendInfo is the extended bag (language follow-up etc.);
	// optional.
	ExtendInfo *CreditApplicationChangeExtendInfo `json:"extendInfo,omitempty"`
}

// CreditApplicationChangeExtendInfo is the optional extendInfo bag
// on /modify-application-info.
type CreditApplicationChangeExtendInfo struct {
	// Language is the optional display language hint for partner-
	// rendered UI follow-ups.
	Language Language `json:"language,omitempty"`
}

// CloseAccountParam is the POST /close-account request body. Just
// the idempotency key + user identifier; no other fields per spec.
type CloseAccountParam struct {
	// RequestID is partner-generated; max 64 chars.
	RequestID string `json:"requestId"`
	// ExternalReferenceUID is the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
}

// ---------- Application-essential-info tree ----------

// ApplicationEssentialInfo is the KYC payload nested inside
// CreditApplicationParam. Field shape may differ per country (today
// only ID); see the request fixture for the canonical sample.
type ApplicationEssentialInfo struct {
	// LivenessCheck carries the liveness snapshot result. Required.
	LivenessCheck *LivenessCheck `json:"livenessCheck"`
	// IndividualProfile carries the user's KYC profile (idType,
	// ocrResult, idFrontPhoto). Required per spec.
	IndividualProfile *IndividualProfile `json:"individualProfile"`
	// PlatformInformation carries partner-side risk signals
	// (creditProfile, sceneType, deviceInfo). Required per spec.
	PlatformInformation *PlatformInformation `json:"platformInformation"`
}

// LivenessCheck is the liveness result on /credit-application.
type LivenessCheck struct {
	Result                string `json:"result"`
	SnapshotPhoto         string `json:"snapshotPhoto"`
	LivenessCheckResult01 string `json:"livenessCheckResult01"`
	LivenessCheckResult02 string `json:"livenessCheckResult02"`
	LivenessCheckResult03 string `json:"livenessCheckResult03"`
}

// IndividualProfile is the user's KYC profile.
type IndividualProfile struct {
	// IDType is the government-issued ID type (e.g. "KTP").
	IDType string `json:"idType"`
	// OCRResult is OCR-extracted fields from the ID document photo.
	OCRResult *OCRResult `json:"ocrResult"`
	// IDFrontPhoto is the base64-encoded front-side photo of the ID
	// document. Required by the spec.
	IDFrontPhoto string `json:"idFrontPhoto"`
}

// OCRResult is the OCR-extracted fields from the ID document photo.
type OCRResult struct {
	IDNumber            string `json:"idNumber"`
	FullName            string `json:"fullName"`
	BirthPlace          string `json:"birthPlace"`
	OCRBloodType        string `json:"ocrBloodType,omitempty"`
	OCRReligion         string `json:"ocrReligion"`
	OCRGender           string `json:"ocrGender"` // enum: MAN | WOMAN
	ManuallyBirthDate   string `json:"manuallyBirthDate"`
	OCRProvince         string `json:"ocrProvince"`
	OCRCity             string `json:"ocrCity"`
	OCRDistrict         string `json:"ocrDistrict"`
	JobType             string `json:"jobType"`
	OCRMaritalStatus    string `json:"ocrMaritalStatus,omitempty"` // SINGLE | MARRIED | DIVORCED | WIDOWED | SEPARATED | OTHERS
	ManuallyExpiredDate string `json:"manuallyExpiredDate"`
	ManuallyCitizenship string `json:"manuallyCitizenship"`
	ManuallyRt          string `json:"manuallyRt"`
	ManuallyRw          string `json:"manuallyRw"`
}

// CreditSceneType is the entry scene on the partner UI.
type CreditSceneType string

const (
	SceneProductLandingPage   CreditSceneType = "PRODUCT_LANDING_PAGE"
	ScenePromoBannerPage      CreditSceneType = "PROMO_BANNER_PAGE"
	SceneMarketingPushEntry   CreditSceneType = "MARKETING_PUSH_ENTRY"
	SceneCheckoutPage         CreditSceneType = "CHECKOUT_PAGE"
	SceneWalletDashboardEntry CreditSceneType = "WALLET_DASHBOARD_ENTRY"
)

// PlatformInformation carries partner-side risk signals.
type PlatformInformation struct {
	// CreditProfile is a JSON string carrying partner risk model
	// scores and engineered features.
	CreditProfile string `json:"creditProfile,omitempty"`
	// SceneType is the UI entry point where the user started credit.
	SceneType CreditSceneType `json:"sceneType"`
	// DeviceInfo is the user's device snapshot at submission time.
	DeviceInfo *DeviceInfo `json:"deviceInfo"`
}

// DeviceInfo is the user's device snapshot.
type DeviceInfo struct {
	// Platform is the device platform (ANDROID | IOS).
	Platform string `json:"platform,omitempty"`
	// GPS is the GPS sample (longitude/latitude/time strings).
	GPS *GPSSample `json:"gps"`
	// Device is the device-build snapshot.
	Device *Device `json:"device,omitempty"`
	// WifiList is the visible Wi-Fi networks at submission time.
	WifiList []WifiAP `json:"wifiList,omitempty"`
	// IPAddress carries ethIp / trueIp.
	IPAddress *IPAddress `json:"ipAddress,omitempty"`
}

// GPSSample is the GPS sample on a device snapshot. Spec types
// longitude/latitude/time as strings (per existing payment.GPS
// pattern; time stays a string here because the credit spec types
// it as string, distinct from /auth's int-Unix-ms).
type GPSSample struct {
	Longitude string `json:"longitude"`
	Latitude  string `json:"latitude"`
	Time      string `json:"time"` // string per credit spec
}

// Device is the device-build snapshot.
type Device struct {
	DeviceID            string       `json:"deviceId"`
	GoogleAdvertisingID string       `json:"googleAdvertisingId,omitempty"`
	IDFA                string       `json:"idfa,omitempty"`
	IDFV                string       `json:"idfv,omitempty"`
	UTDID               string       `json:"utdid"`
	IsRoot              bool         `json:"isRoot,omitempty"`
	AndroidID           string       `json:"androidId,omitempty"`
	Build               *DeviceBuild `json:"build,omitempty"`
}

// DeviceBuild mirrors the Android Build.* fields.
type DeviceBuild struct {
	Board        string `json:"board,omitempty"`
	Brand        string `json:"brand,omitempty"`
	CPUAbi       string `json:"cpuAbi,omitempty"`
	Device       string `json:"device,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	Product      string `json:"product,omitempty"`
}

// WifiAP is one visible Wi-Fi network.
type WifiAP struct {
	SSID string `json:"ssid,omitempty"`
}

// IPAddress is the IP-address bag (ethIp / trueIp).
type IPAddress struct {
	EthIP  string `json:"ethIp,omitempty"`
	TrueIP string `json:"trueIp,omitempty"`
}

// ---------- Response envelopes & data types ----------

// CreditInformationResponse is the POST /credit-information outer
// envelope.
type CreditInformationResponse struct {
	Code    atomefin.Code            `json:"code"`
	Message string                   `json:"message"`
	Data    *CreditInformationResult `json:"data,omitempty"`
}

// CreditInformationResult is the `data` body of
// CreditInformationResponse. Returned by the synchronous
// /credit-information POST.
type CreditInformationResult struct {
	// RequestID echoes the partner's idempotency key.
	RequestID string `json:"requestId"`
	// ExternalReferenceUID echoes the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// Status is the lifecycle state of the credit-information
	// submission (SUCCESS / FAILED / DRAFT).
	Status CreditStatus `json:"status"`
	// JumpURL is the deep-link into Atome's hosted KYC web flow.
	// Required per spec.
	JumpURL string `json:"jumpUrl"`
	// FailReason is set when Status == FAILED.
	FailReason string `json:"failReason,omitempty"`
	// ExtendInfo is the free-form extension bag — kept as a typed
	// object for forward-compat (today empty).
	ExtendInfo *CreditInformationResultExtendInfo `json:"extendInfo,omitempty"`
}

// CreditInformationResultExtendInfo is a placeholder for future
// spec additions on CreditInformationResult.extendInfo. Kept as a
// named type so a minor release can add fields without breaking the
// envelope.
type CreditInformationResultExtendInfo struct{}

// IsSuccess reports whether the envelope's Code is SUCCESS.
// Nil-safe.
func (r *CreditInformationResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// IsTerminal reports whether the inner Status is terminal
// (SUCCESS or FAILED). Nil-safe across the Code/Message/Data spine.
func (r *CreditInformationResponse) IsTerminal() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status.IsTerminal()
}

// CreditApplicationResponse is the POST /credit-application and
// GET /credit-result outer envelope. Also serves as the
// /<creditApplicationNotifyUrl> callback envelope (mirrors the
// refund pattern: synchronous and async share one shape).
type CreditApplicationResponse struct {
	Code    atomefin.Code            `json:"code"`
	Message string                   `json:"message"`
	Data    *CreditApplicationResult `json:"data,omitempty"`
}

// CreditApplicationResult is the `data` body of
// CreditApplicationResponse. The big return shape: enumerates the
// approval outcome, the credit limits, the reapply timestamp, and
// the bill / payment day if approved.
type CreditApplicationResult struct {
	// ExternalReferenceUID echoes the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// Status is the lifecycle state (SUCCESS / FAILED / PROCESSING).
	Status CreditStatus `json:"status"`
	// ReapplyTime is the Unix-ms timestamp at which the user can
	// re-apply if Status == FAILED. Set only on rejection.
	ReapplyTime int64 `json:"reapplyTime,omitempty"`
	// FailReason is the per-factor rejection reasons when
	// Status == FAILED.
	FailReason []FailReason `json:"failReason,omitempty"`
	// Currency is locked to "IDR" today. Required per spec.
	Currency atomefin.Currency `json:"currency"`
	// CreditInfo is the user-account credit snapshot when
	// Status == SUCCESS.
	CreditInfo *CreditInfo `json:"creditInfo,omitempty"`
	// LoanCreditInfo is the loan-side credit snapshot when
	// Status == SUCCESS.
	LoanCreditInfo *LoanCreditInfo `json:"loanCreditInfo,omitempty"`
	// BillDay is the user's bill day-of-month (1–31).
	BillDay int `json:"billDay,omitempty"`
	// PayDay is the user's payment day-of-month (1–31).
	PayDay int `json:"payDay,omitempty"`
	// ExtendInfo is the free-form extension bag (typed for
	// forward-compat; today empty).
	ExtendInfo *CreditApplicationResultExtendInfo `json:"extendInfo,omitempty"`
}

// CreditApplicationResultExtendInfo is a placeholder for future
// spec additions.
type CreditApplicationResultExtendInfo struct{}

// IsSuccess reports whether the envelope's Code is SUCCESS.
// Nil-safe.
func (r *CreditApplicationResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// IsTerminal reports whether the inner Status is terminal
// (SUCCESS or FAILED). Nil-safe across the Code/Message/Data spine.
func (r *CreditApplicationResponse) IsTerminal() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status.IsTerminal()
}

// IsProcessing reports whether the inner Status is PROCESSING.
// Nil-safe.
func (r *CreditApplicationResponse) IsProcessing() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status == CreditStatusProcessing
}

// CreditInfo is the user-account credit snapshot embedded inside
// CreditApplicationResult.
type CreditInfo struct {
	// TotalCredit is the total credit line (minor units).
	// Required per spec.
	TotalCredit atomefin.Amount `json:"totalCredit"`
	// AvailableCredit is the available credit line.
	AvailableCredit atomefin.Amount `json:"availableCredit"`
	// UsedCredit is the used credit line.
	UsedCredit atomefin.Amount `json:"usedCredit"`
	// LateFeeAmount is the accumulated late-fee amount.
	LateFeeAmount atomefin.Amount `json:"lateFeeAmount,omitempty"`
	// OverpaidAmount is the user's overpaid balance.
	OverpaidAmount atomefin.Amount `json:"overpaidAmount,omitempty"`
	// OverpaidWithdrawAmount is the overpaid amount eligible to
	// withdraw.
	OverpaidWithdrawAmount atomefin.Amount `json:"overpaidWithdrawAmount,omitempty"`
	// OverpaidWithdrawLink is the deep-link the partner exposes for
	// the user to withdraw their overpaid balance.
	OverpaidWithdrawLink string `json:"overpaidWithdrawLink,omitempty"`
	// UserStatus is the user-account status (NORMAL /
	// ACCOUNT_BLOCKED_OVERDUE / ACCOUNT_BLOCKED / ACCOUNT_CLOSED).
	UserStatus UserStatus `json:"userStatus"`
	// Version is the snapshot version (Unix-ms).
	Version int64 `json:"version"`
}

// LoanCreditInfo is the loan-side credit snapshot embedded inside
// CreditApplicationResult. Optional — present when status == SUCCESS
// and the loan side has been provisioned.
type LoanCreditInfo struct {
	// LoanStatus is the loan-side lifecycle state (NORMAL /
	// RISK_REJECTED).
	LoanStatus LoanStatus `json:"loanStatus,omitempty"`
	// ReapplyTime is the Unix-ms timestamp at which the loan-side
	// can be re-applied for if rejected.
	ReapplyTime int64 `json:"reapplyTime,omitempty"`
}

// CreditInformationCollectResponse is the GET
// /credit-information-result outer envelope. Also serves as the
// /<creditInformationNotifyUrl> callback envelope.
type CreditInformationCollectResponse struct {
	Code    atomefin.Code                        `json:"code"`
	Message string                               `json:"message"`
	Data    *CreditApplicationCollectQueryResult `json:"data,omitempty"`
}

// CreditApplicationCollectQueryResult is the `data` body of
// CreditInformationCollectResponse. Returned by GET
// /credit-information-result and the credit-information callback.
type CreditApplicationCollectQueryResult struct {
	// RequestID echoes the partner's idempotency key.
	RequestID string `json:"requestId"`
	// ExternalReferenceUID echoes the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// Status is the lifecycle state of the credit-application-collect
	// (SUCCESS / FAILED / DRAFT).
	Status CreditStatus `json:"status"`
	// FailReason is set when Status == FAILED. Spec enumerates a
	// fixed set of values (TOO_MANY_FACES_DETECTED, NO_FACE_DETECTED,
	// INVALID_FACE_DETECTED, FACE_OCCLUDED, FACE_QUALITY_TOO_POOR,
	// LIVENESS_NO_RESULT, SUBMIT_OVER_MAXIMUM_TIME) but the SDK
	// passes the wire literal through as-is to stay forward-compat.
	FailReason string `json:"failReason,omitempty"`
	// ExtendInfo is the free-form extension bag (typed for
	// forward-compat).
	ExtendInfo *CreditCollectQueryResultExtendInfo `json:"extendInfo,omitempty"`
}

// CreditCollectQueryResultExtendInfo is a placeholder for future
// spec additions on CreditApplicationCollectQueryResult.extendInfo.
type CreditCollectQueryResultExtendInfo struct{}

// IsSuccess reports whether the envelope's Code is SUCCESS.
// Nil-safe.
func (r *CreditInformationCollectResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// IsTerminal reports whether the inner Status is terminal.
// Nil-safe.
func (r *CreditInformationCollectResponse) IsTerminal() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status.IsTerminal()
}

// ---------- Balance history ----------

// BalanceHistoryParams are the query params for GET
// /query-balance-history. ExternalReferenceUID and Type are
// required; Start/Count default to 1/10 (server caps Count at 50).
type BalanceHistoryParams struct {
	// ExternalReferenceUID is the partner's user identifier.
	// Required, max 64.
	ExternalReferenceUID string
	// Type is the kind of credit-balance change to query
	// (OVERPAID_CHANGE / CREDIT_LIMIT_ADJUSTMENT /
	// TRADE_AVAILABLE_CREDIT_CHANGE). Required.
	Type BalanceHistoryType
	// RequestID optionally narrows the query to one prior request.
	// Optional, max 64.
	RequestID string
	// Start is the 1-indexed start row. Zero defaults to 1.
	Start int
	// Count is the maximum rows to return. Zero defaults to 10;
	// server cap is 50.
	Count int
}

// BalanceHistoryResponse is the GET /query-balance-history outer
// envelope. The data type is named CreditChangeHistoryResponse per
// spec verbatim — that's a bit awkward (Response-suffixed inside an
// envelope-suffixed Response), but keeping the spec name avoids
// renaming churn for partners reading the swagger.
type BalanceHistoryResponse struct {
	Code    atomefin.Code                `json:"code"`
	Message string                       `json:"message"`
	Data    *CreditChangeHistoryResponse `json:"data,omitempty"`
}

// CreditChangeHistoryResponse is the `data` body — the spec calls
// it CreditChangeHistoryResponse so we keep the name. Holds the
// per-row history list plus a paginator.
//
// RecordInfo is bare `json:"recordInfo"` (no `,omitempty`) per the
// paginated-list pattern: empty pages round-trip as `[]` rather
// than disappearing.
type CreditChangeHistoryResponse struct {
	// ExternalReferenceUID echoes the partner's user identifier.
	// Required.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// Type echoes the queried kind.
	Type BalanceHistoryType `json:"type"`
	// Currency is locked to "IDR" today. Required.
	Currency atomefin.Currency `json:"currency"`
	// RecordInfo is the per-row history list. Bare json (no
	// omitempty) so an empty page emits `[]`.
	RecordInfo []RecordInfo `json:"recordInfo"`
	// Paginator carries start / count / totalCount.
	Paginator *Paginator `json:"paginator,omitempty"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS.
// Nil-safe.
func (r *BalanceHistoryResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// RecordInfo is one row in a CreditChangeHistoryResponse.
type RecordInfo struct {
	// RequestID is the partner-side idempotency key associated with
	// the change.
	RequestID string `json:"requestId,omitempty"`
	// ChangeAmount is the net change amount in minor units. Spec
	// types this as a string — preserved as string here so the
	// SDK's int64-only money policy is not violated by a server
	// that emits "1500000" as a string.
	ChangeAmount string `json:"changeAmount,omitempty"`
	// Event is the trigger for the change (PAYMENT, REPAYMENT,
	// REFUND, REPAYMENT_ATOME, OVERPAID_REPAYMENT,
	// FIXED_CREDIT_LIMIT_BOOST, FIXED_CREDIT_LIMIT_DECREASE,
	// TEMP_CREDIT_LIMIT_BOOST, TEMP_CREDIT_LIMIT_DECREASE,
	// CREDIT_APPLICATION). Spec passes through as a free-form
	// string for forward-compat.
	Event string `json:"event,omitempty"`
	// ValidTil is the Unix-ms expiration of this record (for
	// TEMP_CREDIT_LIMIT_* events).
	ValidTil int64 `json:"validTil,omitempty"`
	// Time is the Unix-ms event time.
	Time int64 `json:"time,omitempty"`
}

// Paginator is the shared start/count/totalCount block on the
// balance-history response.
type Paginator struct {
	Start      int `json:"start,omitempty"`
	Count      int `json:"count,omitempty"`
	TotalCount int `json:"totalCount,omitempty"`
}

// ---------- modify-application-info / close-account responses ----------

// ModifyApplicationInfoResponse is the POST /modify-application-info
// outer envelope. The endpoint returns no `data` per spec — code +
// message only — but the envelope keeps the same Code/Message/Data
// spine so partners can treat all credit responses uniformly.
type ModifyApplicationInfoResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS.
// Nil-safe.
func (r *ModifyApplicationInfoResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// CloseAccountResponse is the POST /close-account outer envelope.
type CloseAccountResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS.
// Nil-safe.
func (r *CloseAccountResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}
