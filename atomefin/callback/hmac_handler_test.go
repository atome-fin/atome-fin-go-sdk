package callback_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

const (
	hmacTestClientID = "grab-client-1"
	hmacTestSecret   = "s3cret-value"
	hmacTestPath     = "/atome/credit-application"
)

// hmacPost builds a POST signed per the OVO.HMAC scheme
// (docs/grab/credit-application-hmac-signing.md).
func hmacPost(t *testing.T, clientID, secret string, body []byte) *http.Request {
	t.Helper()
	timeMs := strconv.FormatInt(time.Now().UnixMilli(), 10)
	canonical := sign.OVOHMACCanonical(clientID, timeMs, http.MethodPost, hmacTestPath, body)
	signer, err := sign.NewOVOHMACSigner(secret)
	if err != nil {
		t.Fatalf("NewOVOHMACSigner: %v", err)
	}
	sig, err := signer.Sign(context.Background(), canonical)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, hmacTestPath, bytes.NewReader(body))
	req.Header.Set("client-id", clientID)
	req.Header.Set("time", timeMs)
	req.Header.Set("signature", sig)
	return req
}

func newHMACVerifier(t *testing.T) *callback.Verifier {
	t.Helper()
	v, err := callback.FromHMACSecret(hmacTestClientID, []string{hmacTestSecret})
	if err != nil {
		t.Fatalf("FromHMACSecret: %v", err)
	}
	if !v.IsHMAC() {
		t.Fatal("IsHMAC() = false, want true")
	}
	return v
}

func TestCreditApplicationHandler_HMAC_HappyPath(t *testing.T) {
	v := newHMACVerifier(t)
	body := creditApplicationCallbackBody()

	var seen *callback.CreditApplicationEvent
	handler := callback.CreditApplicationHandler(v, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
		seen = e
		return nil
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, hmacPost(t, hmacTestClientID, hmacTestSecret, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if seen == nil || seen.ExternalReferenceUID != "user-42" {
		t.Fatalf("event = %#v", seen)
	}
}

func TestCreditApplicationHandler_HMAC_Rejections(t *testing.T) {
	v := newHMACVerifier(t)
	body := creditApplicationCallbackBody()
	nop := func(ctx context.Context, e *callback.CreditApplicationEvent) error { return nil }
	handler := callback.CreditApplicationHandler(v, nop)

	cases := []struct {
		name   string
		mutate func(*http.Request)
	}{
		{"wrong secret", func(r *http.Request) {
			*r = *hmacPost(t, hmacTestClientID, "wrong-secret", body)
		}},
		{"wrong client-id header", func(r *http.Request) {
			r.Header.Set("client-id", "someone-else")
		}},
		{"missing signature", func(r *http.Request) {
			r.Header.Del("signature")
		}},
		{"missing time", func(r *http.Request) {
			r.Header.Del("time")
		}},
		{"tampered body", func(r *http.Request) {
			tampered := bytes.Replace(body, []byte("user-42"), []byte("user-43"), 1)
			r.Body = http.NoBody
			*r = *cloneWithBody(r, tampered)
		}},
		{"rsa2 Authorization header ignored", func(r *http.Request) {
			r.Header.Del("signature")
			r.Header.Set("Authorization", "AAAA")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := hmacPost(t, hmacTestClientID, hmacTestSecret, body)
			tc.mutate(req)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// cloneWithBody swaps the request body while keeping headers (so the
// stale signature no longer matches).
func cloneWithBody(r *http.Request, body []byte) *http.Request {
	req := httptest.NewRequest(r.Method, r.URL.String(), bytes.NewReader(body))
	req.Header = r.Header.Clone()
	return req
}

// Secret rotation: either secret verifies while both are configured.
func TestFromHMACSecret_Rotation(t *testing.T) {
	v, err := callback.FromHMACSecret(hmacTestClientID, []string{"old-secret", hmacTestSecret})
	if err != nil {
		t.Fatalf("FromHMACSecret: %v", err)
	}
	body := creditApplicationCallbackBody()
	nop := func(ctx context.Context, e *callback.CreditApplicationEvent) error { return nil }
	handler := callback.CreditApplicationHandler(v, nop)

	for _, secret := range []string{"old-secret", hmacTestSecret} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, hmacPost(t, hmacTestClientID, secret, body))
		if rec.Code != http.StatusOK {
			t.Fatalf("secret %q: status = %d, body = %s", secret, rec.Code, rec.Body.String())
		}
	}
}
