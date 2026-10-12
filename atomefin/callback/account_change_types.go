package callback

import "github.com/atome-fin/atome-fin-go-sdk/atomefin"

// AccountChangeEventType enumerates the spec's `event` values on
// /account_change_callback.
type AccountChangeEventType string

const (
	AccountEventOverdue               AccountChangeEventType = "ACCOUNT_OVERDUE"
	AccountEventLateFeeIncrease       AccountChangeEventType = "LATE_FEE_INCREASE"
	AccountEventLateFeeDecrease       AccountChangeEventType = "LATE_FEE_DECREASE"
	AccountEventFixedCreditLimitBoost AccountChangeEventType = "FIXED_CREDIT_LIMIT_BOOST"
	AccountEventFixedCreditLimitLower AccountChangeEventType = "FIXED_CREDIT_LIMIT_DECREASE"
	AccountEventTempCreditLimitBoost  AccountChangeEventType = "TEMP_CREDIT_LIMIT_BOOST"
	AccountEventTempCreditLimitLower  AccountChangeEventType = "TEMP_CREDIT_LIMIT_DECREASE"
	AccountEventAtomeControl          AccountChangeEventType = "ATOME_CONTROL"
)

// IsValid reports whether t is a spec-defined account-change event.
// Forward-compat: unknown values round-trip opaquely.
func (t AccountChangeEventType) IsValid() bool {
	switch t {
	case AccountEventOverdue, AccountEventLateFeeIncrease, AccountEventLateFeeDecrease,
		AccountEventFixedCreditLimitBoost, AccountEventFixedCreditLimitLower,
		AccountEventTempCreditLimitBoost, AccountEventTempCreditLimitLower,
		AccountEventAtomeControl:
		return true
	}
	return false
}

// AccountChangeScene enumerates the spec's `scene` values on
// /account_change_callback.
type AccountChangeScene string

const (
	AccountSceneStatusChange          AccountChangeScene = "ACCOUNT_STATUS_CHANGE"
	AccountSceneLateFeeChange         AccountChangeScene = "LATE_FEE_CHANGE"
	AccountSceneOverpaidChange        AccountChangeScene = "OVERPAID_CHANGE"
	AccountSceneCreditLimitAdjustment AccountChangeScene = "CREDIT_LIMIT_ADJUSTMENT"
)

// IsValid reports whether s is a spec-defined account-change scene.
// Forward-compat: unknown values round-trip opaquely.
func (s AccountChangeScene) IsValid() bool {
	switch s {
	case AccountSceneStatusChange, AccountSceneLateFeeChange,
		AccountSceneOverpaidChange, AccountSceneCreditLimitAdjustment:
		return true
	}
	return false
}

// AccountChangeEvent is the flat AccountNotification body posted to
// /account_change_callback.
type AccountChangeEvent struct {
	CallbackRequestID    string                 `json:"callbackRequestId"`
	ExternalReferenceUID string                 `json:"externalReferenceUid"`
	ExternalRequestID    string                 `json:"externalRequestId,omitempty"`
	Event                AccountChangeEventType `json:"event"`
	Scene                AccountChangeScene     `json:"scene"`
	PreviousStatus       atomefin.AccountStatus `json:"previousStatus,omitempty"`
	CurrentStatus        atomefin.AccountStatus `json:"currentStatus,omitempty"`
	Currency             atomefin.Currency      `json:"currency"`
	// AmountChange is the delta for amount-bearing events. Pointer so
	// "absent" decodes as nil, distinct from an explicit 0 (the spec
	// uses 0 to signal a failed CIRCLED_LIST_CREDIT_BOOST).
	AmountChange *atomefin.Amount         `json:"amountChange,omitempty"`
	Version      int64                    `json:"version,omitempty"`
	CreditInfo   *AccountChangeCreditInfo `json:"creditInfo"`
	ExtendInfo   *AccountChangeExtendInfo `json:"extendInfo,omitempty"`
}

// HasIdentity reports whether the event carries its required
// callbackRequestId — a cheap sanity check after decoding.
func (e *AccountChangeEvent) HasIdentity() bool {
	return e != nil && e.CallbackRequestID != ""
}

// IsSuccess is a deprecated alias of HasIdentity. The event body has
// no `code` field; this method never inspected one.
//
// Deprecated: use HasIdentity.
func (e *AccountChangeEvent) IsSuccess() bool { return e.HasIdentity() }

// AccountChangeCreditInfo is the current account snapshot.
type AccountChangeCreditInfo struct {
	TotalCredit     atomefin.Amount        `json:"totalCredit"`
	AvailableCredit atomefin.Amount        `json:"availableCredit"`
	UsedCredit      atomefin.Amount        `json:"usedCredit"`
	LateFeeAmount   atomefin.Amount        `json:"lateFeeAmount,omitempty"`
	OverpaidAmount  atomefin.Amount        `json:"overpaidAmount,omitempty"`
	UserStatus      atomefin.AccountStatus `json:"userStatus"`
}

// AccountChangeExtendInfo carries account notification extension fields.
type AccountChangeExtendInfo struct {
	TempCreditLimitAdjustmentValidTil int64 `json:"tempCreditLimitAdjustmentValidTil,omitempty"`
	// CreditLimitAdjustmentReason is a spec enum (PERFORMANCE_GOOD,
	// PERFORMANCE_BAD, CIRCLED_LIST_CREDIT_BOOST, "RETURN INITIAL",
	// …); kept as string because the spec's own value list contains a
	// space-separated literal and is likely to evolve.
	CreditLimitAdjustmentReason string `json:"creditLimitAdjustmentReason,omitempty"`
}
