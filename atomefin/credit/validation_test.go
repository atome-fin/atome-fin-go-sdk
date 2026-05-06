package credit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
)

// ---------- /credit-information validation ----------

func TestSubmitInformation_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := credit.New(c)

	cases := []struct {
		name      string
		req       *credit.CreditInformationParam
		wantField string
	}{
		{"nil-request", nil, "request"},
		{"long-requestId", &credit.CreditInformationParam{
			RequestID:            strings.Repeat("a", 65),
			ExternalReferenceUID: "u",
			EventType:            credit.EventTypeNewApplication,
			Email:                "e@x",
			Country:              credit.CountryIndonesia,
		}, "requestId"},
		{"long-externalReferenceUid", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: strings.Repeat("u", 65),
			EventType:            credit.EventTypeNewApplication,
			Email:                "e@x",
			Country:              credit.CountryIndonesia,
		}, "externalReferenceUid"},
		{"missing-eventType", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			Email:                "e@x",
			Country:              credit.CountryIndonesia,
		}, "eventType"},
		{"unknown-eventType", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			EventType:            credit.EventType("RENEWAL"),
			Email:                "e@x",
			Country:              credit.CountryIndonesia,
		}, "eventType"},
		{"missing-mobile-when-not-new", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			EventType:            credit.EventTypeSwitchApplication,
			Email:                "e@x",
			Country:              credit.CountryIndonesia,
		}, "mobileNumber"},
		{"missing-email", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			EventType:            credit.EventTypeNewApplication,
			Country:              credit.CountryIndonesia,
		}, "email"},
		{"long-email", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			EventType:            credit.EventTypeNewApplication,
			Email:                strings.Repeat("a", 65),
			Country:              credit.CountryIndonesia,
		}, "email"},
		{"missing-country", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			EventType:            credit.EventTypeNewApplication,
			Email:                "e@x",
		}, "country"},
		{"unsupported-country", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			EventType:            credit.EventTypeNewApplication,
			Email:                "e@x",
			Country:              credit.Country("US"),
		}, "country"},
		{"extendInfo-missing-language", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			EventType:            credit.EventTypeNewApplication,
			Email:                "e@x",
			Country:              credit.CountryIndonesia,
			ExtendInfo:           &credit.CreditInformationExtendInfo{},
		}, "extendInfo.language"},
		{"extendInfo-bad-language", &credit.CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			EventType:            credit.EventTypeNewApplication,
			Email:                "e@x",
			Country:              credit.CountryIndonesia,
			ExtendInfo:           &credit.CreditInformationExtendInfo{Language: credit.Language("zh")},
		}, "extendInfo.language"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.SubmitInformation(context.Background(), tc.req)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- /credit-application validation ----------

func TestSubmitApplication_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := credit.New(c)

	base := func() *credit.CreditApplicationParam { return validApplicationParam() }

	cases := []struct {
		name      string
		mutate    func(*credit.CreditApplicationParam) *credit.CreditApplicationParam
		wantField string
	}{
		{"nil-request", func(_ *credit.CreditApplicationParam) *credit.CreditApplicationParam { return nil }, "request"},
		{"long-requestId", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.RequestID = strings.Repeat("a", 65)
			return req
		}, "requestId"},
		{"missing-mobile", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.MobileNumber = ""
			return req
		}, "mobileNumber"},
		{"missing-email", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.Email = ""
			return req
		}, "email"},
		{"long-email", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.Email = strings.Repeat("a", 65)
			return req
		}, "email"},
		{"unsupported-country", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.Country = "PH"
			return req
		}, "country"},
		{"missing-essentialInfo", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.ApplicationEssentialInfo = nil
			return req
		}, "applicationEssentialInfo"},
		{"missing-individualProfile", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.ApplicationEssentialInfo.IndividualProfile = nil
			return req
		}, "individualProfile"},
		{"missing-platformInformation", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.ApplicationEssentialInfo.PlatformInformation = nil
			return req
		}, "platformInformation"},
		{"out-of-range-userCreditScore", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			score := 1.5
			req.ApplicationEssentialInfo.PlatformInformation.UserCreditScore = &score
			return req
		}, "userCreditScore"},
		{"missing-extendInfo", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.ExtendInfo = nil
			return req
		}, "extendInfo"},
		{"missing-creditInformationRequestId", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.ExtendInfo.CreditInformationRequestID = ""
			return req
		}, "creditInformationRequestId"},
		{"long-creditInformationRequestId", func(req *credit.CreditApplicationParam) *credit.CreditApplicationParam {
			req.ExtendInfo.CreditInformationRequestID = strings.Repeat("a", 65)
			return req
		}, "creditInformationRequestId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := tc.mutate(base())
			_, err := svc.SubmitApplication(context.Background(), req)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- /modify-application-info validation ----------

func TestModifyApplicationInfo_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := credit.New(c)

	cases := []struct {
		name      string
		req       *credit.CreditApplicationChangeParam
		wantField string
	}{
		{"nil-request", nil, "request"},
		// note: missing-requestId is not a rejection case — the
		// service auto-mints when empty (mirrors refund / payment).
		{"long-requestId", &credit.CreditApplicationChangeParam{
			RequestID:            strings.Repeat("a", 65),
			ExternalReferenceUID: "u", MobileNumber: "+62",
		}, "requestId"},
		{"missing-externalReferenceUid", &credit.CreditApplicationChangeParam{
			RequestID: "r", MobileNumber: "+62",
		}, "externalReferenceUid"},
		{"missing-mobileNumber", &credit.CreditApplicationChangeParam{
			RequestID: "r", ExternalReferenceUID: "u",
		}, "mobileNumber"},
		{"long-email", &credit.CreditApplicationChangeParam{
			RequestID: "r", ExternalReferenceUID: "u", MobileNumber: "+62",
			Email: strings.Repeat("a", 65),
		}, "email"},
		{"bad-language", &credit.CreditApplicationChangeParam{
			RequestID: "r", ExternalReferenceUID: "u", MobileNumber: "+62",
			ExtendInfo: &credit.CreditApplicationChangeExtendInfo{Language: credit.Language("zh")},
		}, "language"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ModifyApplicationInfo(context.Background(), tc.req)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- /close-account validation ----------

func TestCloseAccount_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := credit.New(c)

	cases := []struct {
		name      string
		req       *credit.CloseAccountParam
		wantField string
	}{
		{"nil-request", nil, "request"},
		// note: missing-requestId is not a rejection case (auto-mint).
		{"missing-externalReferenceUid", &credit.CloseAccountParam{RequestID: "r"}, "externalReferenceUid"},
		{"long-requestId", &credit.CloseAccountParam{
			RequestID: strings.Repeat("a", 65), ExternalReferenceUID: "u",
		}, "requestId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CloseAccount(context.Background(), tc.req)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- /credit-result + /credit-information-result GET validation ----------

func TestQueryInformationResult_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := credit.New(c)

	if _, err := svc.QueryInformationResult(context.Background(), "", "r"); err == nil {
		t.Error("QueryInformationResult: empty uid must reject")
	} else {
		mustValidationError(t, err, "externalReferenceUid")
	}
	if _, err := svc.QueryInformationResult(context.Background(), strings.Repeat("u", 65), "r"); err == nil {
		t.Error("QueryInformationResult: long uid must reject")
	} else {
		mustValidationError(t, err, "externalReferenceUid")
	}
	if _, err := svc.QueryInformationResult(context.Background(), "u", ""); err == nil {
		t.Error("QueryInformationResult: empty requestId must reject")
	} else {
		mustValidationError(t, err, "requestId")
	}
	if _, err := svc.QueryInformationResult(context.Background(), "u", strings.Repeat("a", 65)); err == nil {
		t.Error("QueryInformationResult: long requestId must reject")
	} else {
		mustValidationError(t, err, "requestId")
	}
}

func TestQueryResult_Validate_LongUID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := credit.New(c)
	_, err := svc.QueryResult(context.Background(), strings.Repeat("u", 65))
	mustValidationError(t, err, "externalReferenceUid")
}

// ---------- /query-balance-history validation ----------

func TestBalanceHistory_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := credit.New(c)

	cases := []struct {
		name      string
		p         *credit.BalanceHistoryParams
		wantField string
	}{
		{"nil-params", nil, "request"},
		{"missing-uid", &credit.BalanceHistoryParams{
			Type: credit.BalanceHistoryTypeOverpaidChange,
		}, "externalReferenceUid"},
		{"long-uid", &credit.BalanceHistoryParams{
			ExternalReferenceUID: strings.Repeat("u", 65),
			Type:                 credit.BalanceHistoryTypeOverpaidChange,
		}, "externalReferenceUid"},
		{"missing-type", &credit.BalanceHistoryParams{
			ExternalReferenceUID: "u",
		}, "type"},
		{"unknown-type", &credit.BalanceHistoryParams{
			ExternalReferenceUID: "u",
			Type:                 credit.BalanceHistoryType("UNKNOWN"),
		}, "type"},
		{"long-requestId", &credit.BalanceHistoryParams{
			ExternalReferenceUID: "u",
			Type:                 credit.BalanceHistoryTypeOverpaidChange,
			RequestID:            strings.Repeat("a", 65),
		}, "requestId"},
		{"negative-start", &credit.BalanceHistoryParams{
			ExternalReferenceUID: "u",
			Type:                 credit.BalanceHistoryTypeOverpaidChange,
			Start:                -1,
		}, "start"},
		{"negative-count", &credit.BalanceHistoryParams{
			ExternalReferenceUID: "u",
			Type:                 credit.BalanceHistoryTypeOverpaidChange,
			Count:                -1,
		}, "count"},
		{"oversize-count", &credit.BalanceHistoryParams{
			ExternalReferenceUID: "u",
			Type:                 credit.BalanceHistoryTypeOverpaidChange,
			Count:                51,
		}, "count"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.BalanceHistory(context.Background(), tc.p)
			mustValidationError(t, err, tc.wantField)
		})
	}
}
