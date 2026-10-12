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
	return hmacPostAt(t, clientID, secret, strconv.FormatInt(time.Now().UnixMilli(), 10), body)
}

// hmacPostAt builds a POST signed for an explicit `time` header
// value, letting replay-window tests pin the timestamp.
func hmacPostAt(t *testing.T, clientID, secret, timeMs string, body []byte) *http.Request {
	t.Helper()
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

// Replay-window checks are opt-in and symmetric: headers too old or
// too far in the future are rejected when WithHMACTimeWindow is set.
func TestCreditApplicationHandler_HMAC_ReplayWindow(t *testing.T) {
	fixedNow := time.UnixMilli(1_700_000_000_000)
	body := creditApplicationCallbackBody()
	nop := func(ctx context.Context, e *callback.CreditApplicationEvent) error { return nil }
	v, err := callback.FromHMACSecret(hmacTestClientID, []string{hmacTestSecret},
		callback.WithHMACTimeWindow(5*time.Minute),
		callback.WithClock(func() time.Time { return fixedNow }))
	if err != nil {
		t.Fatalf("FromHMACSecret: %v", err)
	}
	handler := callback.CreditApplicationHandler(v, nop)

	cases := []struct {
		name   string
		timeMs string
		want   int
	}{
		{"fresh accepted", strconv.FormatInt(fixedNow.UnixMilli()-60_000, 10), http.StatusOK},
		{"too old rejected", strconv.FormatInt(fixedNow.UnixMilli()-6*60_000, 10), http.StatusUnauthorized},
		{"too far future rejected", strconv.FormatInt(fixedNow.UnixMilli()+10*60_000, 10), http.StatusUnauthorized},
		{"non-numeric rejected", "not-a-timestamp", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, hmacPostAt(t, hmacTestClientID, hmacTestSecret, tc.timeMs, body))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// Without WithHMACTimeWindow the timestamp is only part of the
// canonical and is not freshness-checked (legacy behaviour).
func TestCreditApplicationHandler_HMAC_NoReplayWindowByDefault(t *testing.T) {
	v := newHMACVerifier(t)
	body := creditApplicationCallbackBody()
	nop := func(ctx context.Context, e *callback.CreditApplicationEvent) error { return nil }
	handler := callback.CreditApplicationHandler(v, nop)

	old := strconv.FormatInt(time.Now().UnixMilli()-365*24*3600*1000, 10)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, hmacPostAt(t, hmacTestClientID, hmacTestSecret, old, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
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
