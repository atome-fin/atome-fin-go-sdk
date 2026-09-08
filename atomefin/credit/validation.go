package credit

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
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
	if req.ApplicationEssentialInfo.PlatformInformation == nil {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation",
			Message: "required",
		}
	}
	if req.ApplicationEssentialInfo.LivenessCheck != nil || req.ApplicationEssentialInfo.IndividualProfile != nil {
		if req.ApplicationEssentialInfo.LivenessCheck == nil {
			return &atomefin.ValidationError{
				Field:   "applicationEssentialInfo.livenessCheck",
				Message: "required for non-overlap requests",
			}
		}
		if req.ApplicationEssentialInfo.IndividualProfile == nil {
			return &atomefin.ValidationError{
				Field:   "applicationEssentialInfo.individualProfile",
				Message: "required for non-overlap requests",
			}
		}
		if req.ApplicationEssentialInfo.PlatformInformation.LatestGkycTimeStamp == "" {
			return &atomefin.ValidationError{
				Field:   "applicationEssentialInfo.platformInformation.latestGkycTimeStamp",
				Message: "required for non-overlap requests",
			}
		}
	}
	if err := validatePlatformInformation(req.ApplicationEssentialInfo.PlatformInformation); err != nil {
		return err
	}
	if req.ExtendInfo != nil && req.ExtendInfo.Language != "" && !req.ExtendInfo.Language.IsValid() {
		return &atomefin.ValidationError{
			Field:   "extendInfo.language",
			Message: "must be one of en | id",
		}
	}
	return nil
}

func validatePlatformInformation(pi *PlatformInformation) error {
	if pi.SceneType == "" {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.sceneType",
			Message: "required",
		}
	}
	if pi.DeviceInfo == nil {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.deviceInfo",
			Message: "required",
		}
	}
	if !pi.UserFlag.IsValid() {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.userFlag",
			Message: "must be one of OVO | COMMON | GRAB",
		}
	}
	if !pi.SCDUserLevel.IsValid() {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.scdUserLevel",
			Message: "must be one of 1 | 2 | 3",
		}
	}
	if pi.CreditProfile == "" {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.creditProfile",
			Message: "required",
		}
	}
	if pi.DeviceInfo.Platform != "ANDROID" && pi.DeviceInfo.Platform != "IOS" {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.deviceInfo.platform",
			Message: "must be one of ANDROID | IOS",
		}
	}
	if pi.DeviceInfo.Device == nil {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.deviceInfo.device",
			Message: "required",
		}
	}
	if pi.DeviceInfo.Device.GoogleAdvertisingID == "" {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.deviceInfo.device.googleAdvertisingId",
			Message: "required",
		}
	}
	if pi.DeviceInfo.Device.Build == nil {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.deviceInfo.device.build",
			Message: "required",
		}
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"deviceId", pi.DeviceInfo.Device.DeviceID},
		{"googleAdvertisingId", pi.DeviceInfo.Device.GoogleAdvertisingID},
		{"build.board", pi.DeviceInfo.Device.Build.Board},
		{"build.brand", pi.DeviceInfo.Device.Build.Brand},
		{"build.device", pi.DeviceInfo.Device.Build.Device},
		{"build.manufacturer", pi.DeviceInfo.Device.Build.Manufacturer},
		{"build.model", pi.DeviceInfo.Device.Build.Model},
		{"build.product", pi.DeviceInfo.Device.Build.Product},
	} {
		if field.value == "" {
			return &atomefin.ValidationError{
				Field:   "applicationEssentialInfo.platformInformation.deviceInfo.device." + field.name,
				Message: "required",
			}
		}
	}
	if pi.DeviceInfo.WifiList == nil {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.deviceInfo.wifiList",
			Message: "required",
		}
	}
	for _, wifi := range pi.DeviceInfo.WifiList {
		if wifi.SSID == "" {
			return &atomefin.ValidationError{
				Field:   "applicationEssentialInfo.platformInformation.deviceInfo.wifiList[].ssid",
				Message: "required",
			}
		}
	}
	if pi.DeviceInfo.IPAddress == nil {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.deviceInfo.ipAddress",
			Message: "required",
		}
	}
	if pi.DeviceInfo.IPAddress.EthIP == "" || pi.DeviceInfo.IPAddress.TrueIP == "" {
		return &atomefin.ValidationError{
			Field:   "applicationEssentialInfo.platformInformation.deviceInfo.ipAddress.ethIp/trueIp",
			Message: "both fields are required",
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

func validateOCRResult(ocr *OCRResult) error {
	required := []struct {
		field string
		value string
	}{
		{"idNumber", ocr.IDNumber},
		{"fullName", ocr.FullName},
		{"birthPlace", ocr.BirthPlace},
		{"manuallyBirthDate", ocr.ManuallyBirthDate},
		{"jobType", ocr.JobType},
		{"ocrProvince", ocr.OCRProvince},
		{"ocrCity", ocr.OCRCity},
		{"ocrDistrict", ocr.OCRDistrict},
		{"ocrGender", ocr.OCRGender},
		{"ocrReligion", ocr.OCRReligion},
		{"manuallyRt", ocr.ManuallyRt},
		{"manuallyRw", ocr.ManuallyRw},
		{"manuallyExpiredDate", ocr.ManuallyExpiredDate},
		{"manuallyCitizenship", ocr.ManuallyCitizenship},
	}
	for _, r := range required {
		if r.value == "" {
			return &atomefin.ValidationError{
				Field:   "applicationEssentialInfo.individualProfile.ocrResult." + r.field,
				Message: "required",
			}
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
