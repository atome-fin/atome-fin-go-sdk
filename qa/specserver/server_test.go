package specserver_test

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// ---------- Loader / parser ----------

func TestLoadDefault_PinnedSpec_ParsesAllOps(t *testing.T) {
	s, err := specserver.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	// Spot-check a handful of operations the SDK must call. If the
	// pinned spec is bumped and these op-keys disappear, the
	// failure should localize the regression to a specific
	// rename / removal.
	wantOps := []string{
		"POST /auth",
		"POST /capture",
		"POST /voidAuth",
		"POST /refund",
		"GET /query-auth",
		"GET /query-capture",
		"GET /query-voidAuth",
		"GET /query-refund",
		"GET /heart-beat",
		"POST /credit-information",
		"POST /credit-application",
		"GET /credit-result",
		"POST /repayment-request",
		"GET /repayment-result",
	}
	for _, want := range wantOps {
		parts := strings.SplitN(want, " ", 2)
		if _, ok := s.Op(parts[0], parts[1]); !ok {
			t.Errorf("expected op %q in pinned spec, not present", want)
		}
	}
}

func TestLoadDefault_QueryRequired_BothKeys(t *testing.T) {
	s, err := specserver.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	// The bug class this framework exists to catch: GET endpoints
	// where BOTH requestId AND externalReferenceUid are required.
	// /query-auth, /query-capture, /query-voidAuth, /query-refund
	// must all carry both names in RequiredQuery.
	for _, op := range []string{
		"GET /query-auth",
		"GET /query-capture",
		"GET /query-voidAuth",
		"GET /query-refund",
	} {
		parts := strings.SplitN(op, " ", 2)
		got, ok := s.Op(parts[0], parts[1])
		if !ok {
			t.Errorf("%s: missing", op)
			continue
		}
		hasReq := contains(got.RequiredQuery, "requestId")
		hasExt := contains(got.RequiredQuery, "externalReferenceUid")
		if !hasReq || !hasExt {
			t.Errorf("%s RequiredQuery=%v; want both requestId and externalReferenceUid",
				op, got.RequiredQuery)
		}
	}
}

func TestLoadDefault_BodyRequired_NestedSubOrders(t *testing.T) {
	s, err := specserver.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	// /auth's body has nested required fields on subOrders[]
	// elements. The walker resolves the SubOrder $ref and the
	// per-element required-set.
	auth, ok := s.Op("POST", "/auth")
	if !ok {
		t.Fatal("POST /auth missing")
	}
	for _, want := range []string{
		"externalReferenceUid",
		"requestId",
		"totalAmount",
		"subOrders[].subOrderId",
		"subOrders[].amount",
		"subOrders[].quantity",
	} {
		if !contains(auth.RequiredBody, want) {
			t.Errorf("POST /auth body missing required path %q (got %v)", want, auth.RequiredBody)
		}
	}
}

// ---------- Server dispatch (validation flow) ----------

func TestServer_GET_RejectsMissingRequiredQuery(t *testing.T) {
	srv := specserver.New(t)

	// Build a GET to /query-auth WITHOUT externalReferenceUid.
	resp, err := http.Get(srv.URL + "/query-auth?requestId=r-1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	failures := srv.Failures()
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	f := failures[0]
	if f.Op != "GET /query-auth" || f.Field != "externalReferenceUid" {
		t.Errorf("Failure = %+v; want op=GET /query-auth, field=externalReferenceUid", f)
	}
}

func TestServer_GET_AcceptsBothRequiredQuery(t *testing.T) {
	srv := specserver.New(t)
	resp, err := http.Get(srv.URL + "/query-auth?" + url.Values{
		"requestId":            []string{"r-1"},
		"externalReferenceUid": []string{"u-1"},
	}.Encode())
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := len(srv.Failures()); got != 0 {
		t.Errorf("failures = %d, want 0", got)
	}
	if srv.Hits("GET /query-auth") != 1 {
		t.Errorf("hits = %d, want 1", srv.Hits("GET /query-auth"))
	}
}

func TestServer_POST_RejectsMissingTopLevelRequired(t *testing.T) {
	srv := specserver.New(t)

	// Body missing externalReferenceUid (and other required
	// fields). The first miss reported should be alphabetically
	// first in the spec's required set.
	body := strings.NewReader(`{"requestId":"r-1"}`)
	resp, err := http.Post(srv.URL+"/voidAuth", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	failures := srv.Failures()
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if failures[0].Reason != "missing required body field" {
		t.Errorf("reason = %q", failures[0].Reason)
	}
}

func TestServer_POST_RejectsMissingNestedRequired(t *testing.T) {
	srv := specserver.New(t)

	// /auth body present at top level but a subOrders[] element
	// missing subOrderId. The walker's nested validation should
	// flag this as missing "subOrders[0].subOrderId".
	body := strings.NewReader(`{
		"requestId":"r-1",
		"externalReferenceUid":"u-1",
		"totalAmount":1000,
		"periodType":1,
		"subOrders":[{"amount":1000,"quantity":1}]
	}`)
	resp, err := http.Post(srv.URL+"/auth", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	failures := srv.Failures()
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if !strings.Contains(failures[0].Field, "subOrderId") {
		t.Errorf("field = %q; want subOrderId reference", failures[0].Field)
	}
}

func TestServer_POST_AcceptsCompleteBody(t *testing.T) {
	srv := specserver.New(t)
	body := strings.NewReader(`{
		"requestId":"r-1",
		"externalReferenceUid":"u-1",
		"authOrderId":"a-1"
	}`)
	resp, err := http.Post(srv.URL+"/voidAuth", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := len(srv.Failures()); got != 0 {
		t.Errorf("failures = %d, want 0", got)
	}
}

func TestServer_RejectsUnknownOperation(t *testing.T) {
	srv := specserver.New(t)
	resp, err := http.Get(srv.URL + "/no-such-endpoint")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServer_FixtureLoad_RoundTrip(t *testing.T) {
	srv := specserver.New(t)

	// Use an existing fixture so we don't ship a duplicate.
	fixture, ferr := findRepoFile("qa/testdata/voidAuth_response_success.json")
	if ferr != nil {
		t.Skipf("fixture not present: %v", ferr)
	}
	if err := srv.SetFixture("POST /voidAuth", fixture); err != nil {
		t.Fatalf("SetFixture: %v", err)
	}

	body := strings.NewReader(`{
		"requestId":"r-1",
		"externalReferenceUid":"u-1",
		"authOrderId":"a-1"
	}`)
	resp, err := http.Post(srv.URL+"/voidAuth", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q", got)
	}
}

// ---------- End-to-end signed Client → server flow ----------

func TestClient_DoSignedGET_MissingExtUID_FailsValidation(t *testing.T) {
	// This is the framework's signal-path smoke test: drive a
	// real signed GET that intentionally omits externalReferenceUid
	// (the §2.1 bug class) and confirm the spec server records the
	// missing-required failure with the right field name. This is
	// the precise signal RunCases surfaces in real per-package
	// tests; here we exercise the underlying Client→server path
	// without the wrapping t.Run scaffolding.
	srv := specserver.New(t)
	c := specserver.MustClient(t, srv)
	_, err := c.DoSignedGET(context.Background(), "/query-auth", url.Values{
		"requestId": []string{"r-1"},
	})
	if err == nil {
		t.Fatal("DoSignedGET succeeded; want APIError from spec-validation")
	}
	failures := srv.Failures()
	if len(failures) != 1 {
		t.Fatalf("failures = %d; want 1: %+v", len(failures), failures)
	}
	if failures[0].Op != "GET /query-auth" || failures[0].Field != "externalReferenceUid" {
		t.Errorf("Failure = %+v; want op=GET /query-auth field=externalReferenceUid", failures[0])
	}
	if !strings.Contains(failures[0].SpecPath, "swagger-") {
		t.Errorf("SpecPath = %q; want pinned-swagger reference", failures[0].SpecPath)
	}
}

// ---------- helpers ----------

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func findRepoFile(rel string) (string, error) {
	// Walk up from CWD until we find go.mod, then resolve rel.
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			candidate := filepath.Join(dir, rel)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
			return "", os.ErrNotExist
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
