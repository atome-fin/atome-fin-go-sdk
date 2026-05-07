package mock_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/mock"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// fakeTB is a tiny *testing.TB substitute used to capture
// `t.Fatalf` calls without aborting the outer test. Specifically
// used to verify the EnvProd refusal guard fires.
type fakeTB struct {
	testing.TB
	fatal []string
	err   []string
}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.fatal = append(f.fatal, sprintf(format, args...))
	// Don't actually exit — we want the outer test to keep running.
}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.err = append(f.err, sprintf(format, args...))
}

func (f *fakeTB) Helper()           {}
func (f *fakeTB) Cleanup(_ func())  {}
func (f *fakeTB) Fatal(args ...any) { f.fatal = append(f.fatal, sprint(args...)) }

func sprintf(f string, a ...any) string {
	// Tiny inline avoid pulling fmt into the fake-tb path.
	// Real tests below use t.* normally; this stays small.
	return f
}

func sprint(a ...any) string {
	return ""
}

// ---------- EnvProd refusal — the #1 risk-class guard ----------

func TestNewClient_RefusesEnvProd(t *testing.T) {
	ftb := &fakeTB{}
	mock.NewClient(ftb,
		mock.WithEnvironment(atomefin.EnvProd),
		mock.WithMockKeysAllowed(),
	)
	if len(ftb.fatal) == 0 {
		t.Fatal("expected t.Fatalf for EnvProd; got none")
	}
	joined := strings.Join(ftb.fatal, "\n")
	if !strings.Contains(joined, "EnvProd") {
		t.Errorf("Fatalf message missing EnvProd reference: %s", joined)
	}
	if !strings.Contains(joined, "REFUSED") {
		t.Errorf("Fatalf message missing REFUSED token: %s", joined)
	}
}

func TestNewClient_AcceptsEnvTest(t *testing.T) {
	c := mock.NewClient(t, mock.WithMockKeysAllowed())
	if c == nil {
		t.Fatal("nil client")
	}
	if c.Environment() != atomefin.EnvTest {
		t.Errorf("Environment = %q, want %q", c.Environment(), atomefin.EnvTest)
	}
}

func TestNewClient_AcceptsEnvPre(t *testing.T) {
	c := mock.NewClient(t,
		mock.WithEnvironment(atomefin.EnvPre),
		mock.WithMockKeysAllowed(),
	)
	if c == nil {
		t.Fatal("nil client")
	}
	if c.Environment() != atomefin.EnvPre {
		t.Errorf("Environment = %q, want %q", c.Environment(), atomefin.EnvPre)
	}
}

// ---------- Scenario dispatch ----------

func TestNewClient_DefaultScenario_AlwaysSuccess(t *testing.T) {
	c := mock.NewClient(t, mock.WithMockKeysAllowed())
	resp, err := payment.New(c).Auth(context.Background(), &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1500000, Quantity: 1}},
		Sessionid:            "s",
	})
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if resp.Code != atomefin.CodeSuccess {
		t.Errorf("Code = %q, want SUCCESS", resp.Code)
	}
}

func TestNewClient_AlwaysAPIError_400(t *testing.T) {
	c := mock.NewClient(t,
		mock.WithMockKeysAllowed(),
		mock.WithScenario(mock.AlwaysAPIError(400, atomefin.CodeParamsMissing, "missing requestId")),
	)
	_, err := payment.New(c).Auth(context.Background(), &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1500000, Quantity: 1}},
		Sessionid:            "s",
	})
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeParamsMissing {
		t.Errorf("Code = %q", ae.Code)
	}
	if !strings.Contains(ae.Message, "missing requestId") {
		t.Errorf("Message = %q", ae.Message)
	}
}

func TestNewClient_PerEndpoint_RoutesByOp(t *testing.T) {
	c := mock.NewClient(t,
		mock.WithMockKeysAllowed(),
		mock.WithScenario(mock.PerEndpoint(map[string]mock.Scenario{
			"POST /auth":    mock.AlwaysSuccess(),
			"POST /capture": mock.AlwaysAPIError(400, atomefin.CodeParamsMissing, "captureRequestId required"),
		}, mock.AlwaysSuccess())),
	)
	authResp, err := payment.New(c).Auth(context.Background(), validAuth())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if authResp.Code != atomefin.CodeSuccess {
		t.Errorf("auth.Code = %q", authResp.Code)
	}
	_, err = payment.New(c).Capture(context.Background(), &payment.CaptureRequest{
		RequestID:            "c-1",
		ExternalReferenceUID: "u-1",
		AuthOrderID:          "AUTH-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1500000, Quantity: 1}},
	})
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("Capture err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeParamsMissing {
		t.Errorf("Capture Code = %q", ae.Code)
	}
}

func TestTransport_RecordsHits(t *testing.T) {
	transport := mock.NewTransport(t, mock.AlwaysSuccess())
	c, err := atomefin.New(
		atomefin.WithBaseURL("https://atome-fin.test"),
		atomefin.WithPrivateKeyPEM(mock.MockSigningPrivKeyPEM()),
		atomefin.WithHTTPClient(&http.Client{Transport: transport}),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	for i := 0; i < 3; i++ {
		_, err := payment.New(c).Auth(context.Background(), validAuth())
		if err != nil {
			t.Fatalf("Auth #%d: %v", i+1, err)
		}
	}

	if got := transport.Hits("POST /auth"); got != 3 {
		t.Errorf("Hits(POST /auth) = %d, want 3", got)
	}
	if got := transport.Hits("POST /capture"); got != 0 {
		t.Errorf("Hits(POST /capture) = %d, want 0", got)
	}
	if got := len(transport.Requests()); got != 3 {
		t.Errorf("len(Requests) = %d, want 3", got)
	}
}

func TestTransport_RecordsBody(t *testing.T) {
	transport := mock.NewTransport(t, mock.AlwaysSuccess())
	c, err := atomefin.New(
		atomefin.WithBaseURL("https://atome-fin.test"),
		atomefin.WithPrivateKeyPEM(mock.MockSigningPrivKeyPEM()),
		atomefin.WithHTTPClient(&http.Client{Transport: transport}),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	_, err = payment.New(c).Auth(context.Background(), validAuth())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}

	reqs := transport.Requests()
	if len(reqs) != 1 {
		t.Fatalf("len(Requests) = %d", len(reqs))
	}
	if !strings.Contains(string(reqs[0].Body), `"requestId":"r-mock-1"`) {
		t.Errorf("body missing requestId: %s", reqs[0].Body)
	}
}

func TestTransport_SetScenario_SwapsBetweenCalls(t *testing.T) {
	transport := mock.NewTransport(t, mock.AlwaysProcessing())
	c, err := atomefin.New(
		atomefin.WithBaseURL("https://atome-fin.test"),
		atomefin.WithPrivateKeyPEM(mock.MockSigningPrivKeyPEM()),
		atomefin.WithHTTPClient(&http.Client{Transport: transport}),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	resp, err := payment.New(c).Auth(context.Background(), validAuth())
	if err != nil {
		t.Fatalf("Auth #1: %v", err)
	}
	if !resp.IsProcessing() {
		t.Errorf("expected PROCESSING; got Data=%#v", resp.Data)
	}

	transport.SetScenario(mock.AlwaysSuccess())
	resp2, err := payment.New(c).Auth(context.Background(), validAuth())
	if err != nil {
		t.Fatalf("Auth #2: %v", err)
	}
	if resp2.Code != atomefin.CodeSuccess {
		t.Errorf("after swap Code = %q", resp2.Code)
	}
}

// ---------- helpers ----------

func validAuth() *payment.AuthRequest {
	return &payment.AuthRequest{
		RequestID:            "r-mock-1",
		ExternalReferenceUID: "u-mock-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1500000, Quantity: 1}},
		Sessionid:            "s",
	}
}
