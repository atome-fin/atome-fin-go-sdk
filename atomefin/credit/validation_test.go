package credit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
)

// /credit-information and /credit-application validators are
// preserved in validation.go for v0.3 re-enablement but no longer
// reached by SubmitInformation / SubmitApplication (both blocked
// in v0.2.x — see TestSubmitInformation_BlockedUntilV0_3 in
// service_test.go). The validator-internal tests live in
// validation_internal_test.go (white-box, package credit) where
// they exercise validateCreditInformation / validateCreditApplication
// directly without going through the blocked public methods.

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
