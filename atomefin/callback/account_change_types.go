package callback

import "github.com/atome-fin/atome-fin-go-sdk/atomefin"

// AccountChangeEvent is the flat AccountNotification body posted to
// /account_change_callback.
type AccountChangeEvent struct {
	CallbackRequestID    string                   `json:"callbackRequestId"`
	ExternalReferenceUID string                   `json:"externalReferenceUid"`
	ExternalRequestID    string                   `json:"externalRequestId,omitempty"`
	Event                string                   `json:"event"`
	Scene                string                   `json:"scene"`
	PreviousStatus       atomefin.AccountStatus   `json:"previousStatus,omitempty"`
	CurrentStatus        atomefin.AccountStatus   `json:"currentStatus,omitempty"`
	Currency             atomefin.Currency        `json:"currency"`
	AmountChange         atomefin.Amount          `json:"amountChange,omitempty"`
	Version              int64                    `json:"version,omitempty"`
	CreditInfo           *AccountChangeCreditInfo `json:"creditInfo"`
	ExtendInfo           *AccountChangeExtendInfo `json:"extendInfo,omitempty"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS.
// Nil-safe.
func (e *AccountChangeEvent) IsSuccess() bool {
	return e != nil && e.CallbackRequestID != ""
}

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
	TempCreditLimitAdjustmentValidTil int64  `json:"tempCreditLimitAdjustmentValidTil,omitempty"`
	CreditLimitAdjustmentReason       string `json:"creditLimitAdjustmentReason,omitempty"`
}
