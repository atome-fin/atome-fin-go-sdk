package payment_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/qa/marshal"
)

func TestService_Riplay_Success(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"success","data":{"tenor":3,"url":"https://cdn.example.com/riplay/SES-1/3.pdf"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).Riplay(context.Background(), &payment.RiplayRequest{
		SessionID:            "SES-1",
		ExternalReferenceUID: "u-1",
		Tenor:                3,
	})
	if err != nil {
		t.Fatalf("Riplay: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
	if resp.Data == nil || resp.Data.Tenor != 3 || resp.Data.URL == "" {
		t.Errorf("Data = %#v", resp.Data)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q", gotMethod)
	}
	if gotPath != "/riplay" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(string(gotBody), `"sessionId":"SES-1"`) {
		t.Errorf("body = %s", gotBody)
	}
}

func TestService_Riplay_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"SESSION_NOT_FOUND","message":"expired"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := payment.New(c).Riplay(context.Background(), &payment.RiplayRequest{
		SessionID:            "SES-expired",
		ExternalReferenceUID: "u-1",
		Tenor:                3,
	})
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeSessionNotFound {
		t.Errorf("Code = %q", ae.Code)
	}
}

func TestRiplay_Validate_TableDriven(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on validation failure")
	})))
	svc := payment.New(c)

	cases := []struct {
		name      string
		req       *payment.RiplayRequest
		wantField string
	}{
		{"nil-request", nil, "request"},
		{"missing-sessionId", &payment.RiplayRequest{
			ExternalReferenceUID: "u", Tenor: 3,
		}, "sessionId"},
		{"long-sessionId", &payment.RiplayRequest{
			SessionID: strings.Repeat("s", 65), ExternalReferenceUID: "u", Tenor: 3,
		}, "sessionId"},
		{"missing-externalReferenceUid", &payment.RiplayRequest{
			SessionID: "SES-1", Tenor: 3,
		}, "externalReferenceUid"},
		{"long-externalReferenceUid", &payment.RiplayRequest{
			SessionID: "SES-1", ExternalReferenceUID: strings.Repeat("u", 65), Tenor: 3,
		}, "externalReferenceUid"},
		{"zero-tenor", &payment.RiplayRequest{
			SessionID: "SES-1", ExternalReferenceUID: "u",
		}, "tenor"},
		{"invalid-tenor", &payment.RiplayRequest{
			SessionID: "SES-1", ExternalReferenceUID: "u", Tenor: 2,
		}, "tenor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Riplay(context.Background(), tc.req)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

func TestRiplayRequest_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[payment.RiplayRequest](t, fixtureRoot+"riplay_request.json")
}

func TestRiplayResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[payment.RiplayResponse](t, fixtureRoot+"riplay_response.json")
}
