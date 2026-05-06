package credit

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// Spec-stamped maxLength constraints. Used by the validators to
// match the swagger.yaml-declared schema bounds; unstamped fields
// (e.g. mobileNumber) are not bounded by the SDK so the spec can
// evolve without an SDK rebuild.
const (
	maxRequestID            = 64
	maxExternalReferenceUID = 64
	maxEmail                = 64
)

// validateCreditInformation is the small client-side guard for
// POST /credit-information. Server-level validation still rules;
// this lets partners surface common mistakes locally without paying
// the network round-trip.
func validateCreditInformation(req *CreditInformationParam) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "required"}
	}
	if len(req.RequestID) > maxRequestID {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if len(req.ExternalReferenceUID) > maxExternalReferenceUID {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "exceeds spec maxlength 64"}
	}
	if req.EventType == "" {
		return &atomefin.ValidationError{Field: "eventType", Message: "required"}
	}
	// Permissive enum check: warn-but-pass on unknown values
	// (mirrors atomefin.Currency.IsValid pattern). Strict rejection
	// would block forward-compat additions like a new event-type
	// the spec adds before the SDK rebuilds.
	if req.EventType != EventTypeNewApplication && req.EventType != EventTypeSwitchApplication {
		return &atomefin.ValidationError{
			Field:   "eventType",
			Message: "must be one of NEW_APPLICATION | SWITCH_APPLICATION",
		}
	}
	if req.EventType != EventTypeNewApplication && req.MobileNumber == "" {
		return &atomefin.ValidationError{
			Field:   "mobileNumber",
			Message: "required when eventType != NEW_APPLICATION",
		}
	}
	if req.Email == "" {
		return &atomefin.ValidationError{Field: "email", Message: "required"}
	}
	if len(req.Email) > maxEmail {
		return &atomefin.ValidationError{Field: "email", Message: "exceeds spec maxlength 64"}
	}
	if req.Country == "" {
		return &atomefin.ValidationError{Field: "country", Message: "required"}
	}
	if !req.Country.IsValid() {
		return &atomefin.ValidationError{
			Field:   "country",
			Message: "only ID is currently supported by the spec",
		}
	}
	if req.ExtendInfo != nil {
		if req.ExtendInfo.Language == "" {
			return &atomefin.ValidationError{
				Field:   "extendInfo.language",
				Message: "required when extendInfo is present",
			}
		}
		if !req.ExtendInfo.Language.IsValid() {
			return &atomefin.ValidationError{
				Field:   "extendInfo.language",
				Message: "must be one of en | id",
			}
		}
	}
	return nil
}

// validateCreditApplication is the client-side guard for POST
// /credit-application.
func validateCreditApplication(req *CreditApplicationParam) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "required"}
	}
	if len(req.RequestID) > maxRequestID {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if len(req.ExternalReferenceUID) > maxExternalReferenceUID {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "exceeds spec maxlength 64"}
	}
	if req.MobileNumber == "" {
		return &atomefin.ValidationError{Field: "mobileNumber", Message: "required"}
	}
	if req.Email == "" {
		return &atomefin.ValidationError{Field: "email", Message: "required"}
	}
	if len(req.Email) > maxEmail {
		return &atomefin.ValidationError{Field: "email", Message: "exceeds spec maxlength 64"}
	}
	if req.Country == "" {
		return &atomefin.ValidationError{Field: "country", Message: "required"}
	}
	if !req.Country.IsValid() {
		return &atomefin.ValidationError{
			Field:   "country",
			Message: "only ID is currently supported by the spec",
		}
	}
	if req.ApplicationEssentialInfo == nil {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo",
			Message: "required",
		}
	}
	if req.ApplicationEssentialInfo.IndividualProfile == nil {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.individualProfile",
			Message: "required",
		}
	}
	if req.ApplicationEssentialInfo.PlatformInformation == nil {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation",
			Message: "required",
		}
	}
	// userCreditScore is in [0, 1] when set; reuse payment.IsValidScore
	// so the rule cannot drift between credit-application and
	// auth/capture/precheck/plan validators.
	if pi := req.ApplicationEssentialInfo.PlatformInformation; pi != nil {
		if !payment.IsValidScore(pi.UserCreditScore) {
			return &atomefin.ValidationError{
				Field:   "applicationEssentialInfo.platformInformation.userCreditScore",
				Message: "must be in [0, 1] (per spec)",
			}
		}
	}
	if req.ExtendInfo == nil {
		return &atomefin.ValidationError{
			Field:   "extendInfo",
			Message: "required (carries creditInformationRequestId)",
		}
	}
	if req.ExtendInfo.CreditInformationRequestID == "" {
		return &atomefin.ValidationError{
			Field:   "extendInfo.creditInformationRequestId",
			Message: "required (the requestId from the prior /credit-information call)",
		}
	}
	if len(req.ExtendInfo.CreditInformationRequestID) > maxRequestID {
		return &atomefin.ValidationError{
			Field:   "extendInfo.creditInformationRequestId",
			Message: "exceeds spec maxlength 64",
		}
	}
	return nil
}

// validateCreditApplicationChange is the client-side guard for POST
// /modify-application-info.
func validateCreditApplicationChange(req *CreditApplicationChangeParam) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "required"}
	}
	if len(req.RequestID) > maxRequestID {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if len(req.ExternalReferenceUID) > maxExternalReferenceUID {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "exceeds spec maxlength 64"}
	}
	if req.MobileNumber == "" {
		return &atomefin.ValidationError{
			Field:   "mobileNumber",
			Message: "required (per spec — the modify endpoint always carries the new mobile)",
		}
	}
	if req.Email != "" && len(req.Email) > maxEmail {
		return &atomefin.ValidationError{Field: "email", Message: "exceeds spec maxlength 64"}
	}
	if req.ExtendInfo != nil && req.ExtendInfo.Language != "" && !req.ExtendInfo.Language.IsValid() {
		return &atomefin.ValidationError{
			Field:   "extendInfo.language",
			Message: "must be one of en | id",
		}
	}
	return nil
}

// validateCloseAccount is the client-side guard for POST
// /close-account.
func validateCloseAccount(req *CloseAccountParam) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "required"}
	}
	if len(req.RequestID) > maxRequestID {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if len(req.ExternalReferenceUID) > maxExternalReferenceUID {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "exceeds spec maxlength 64"}
	}
	return nil
}

// validateBalanceHistoryParams is the client-side guard for GET
// /query-balance-history.
func validateBalanceHistoryParams(p *BalanceHistoryParams) error {
	if p.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if len(p.ExternalReferenceUID) > maxExternalReferenceUID {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "exceeds spec maxlength 64"}
	}
	if p.Type == "" {
		return &atomefin.ValidationError{Field: "type", Message: "required"}
	}
	if !p.Type.IsValid() {
		return &atomefin.ValidationError{
			Field:   "type",
			Message: "must be one of OVERPAID_CHANGE | CREDIT_LIMIT_ADJUSTMENT | TRADE_AVAILABLE_CREDIT_CHANGE",
		}
	}
	if p.RequestID != "" && len(p.RequestID) > maxRequestID {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if p.Start < 0 {
		return &atomefin.ValidationError{Field: "start", Message: "must be >= 0 (0 → default 1)"}
	}
	if p.Count < 0 {
		return &atomefin.ValidationError{Field: "count", Message: "must be >= 0 (0 → default 10)"}
	}
	if p.Count > MaxCount {
		return &atomefin.ValidationError{
			Field:   "count",
			Message: "exceeds spec server cap of 50",
		}
	}
	return nil
}
