package atomefin

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type countingSigner struct{ calls int }

func (s *countingSigner) Sign(ctx context.Context, _ []byte) (string, error) {
	s.calls++
	return "test-signature", ctx.Err()
}

func (*countingSigner) KeyID() string { return "" }

func testHTTPResponse(status int, body io.ReadCloser) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: body}
}

func TestWithRetryDefaultsMissingCallbacks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		err    error
	}{
		{name: "success", status: http.StatusOK},
		{name: "server error", status: http.StatusServiceUnavailable},
		{name: "network error", err: io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c, err := New(WithSigner(&countingSigner{}), WithBaseURL("https://test.invalid"),
				WithRetry(transport.RetryPolicy{MaxAttempts: 1}),
				WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					if tc.err != nil {
						return nil, tc.err
					}
					return testHTTPResponse(tc.status, io.NopCloser(strings.NewReader(`{}`))), nil
				})}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.DoSigned(context.Background(), http.MethodPost, "/auth", []byte(`{}`))
			wantErr := tc.err != nil || tc.status >= 400
			if (err != nil) != wantErr || calls != 1 {
				t.Fatalf("calls=%d err=%v; want one attempt, error=%v", calls, err, wantErr)
			}
		})
	}
}

func TestWithRetryPreservesCustomCallbacks(t *testing.T) {
	for _, networkError := range []bool{false, true} {
		t.Run(map[bool]string{false: "status", true: "transport"}[networkError], func(t *testing.T) {
			calls, policyCalls := 0, 0
			policy := transport.RetryPolicy{MaxAttempts: 3}
			if networkError {
				policy.RetryOnTransportError = func(error) bool { policyCalls++; return false }
			} else {
				policy.RetryOnStatus = func(int) bool { policyCalls++; return false }
			}
			c, err := New(WithSigner(&countingSigner{}), WithBaseURL("https://test.invalid"), WithRetry(policy),
				WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					if networkError {
						return nil, io.EOF
					}
					return testHTTPResponse(http.StatusServiceUnavailable, io.NopCloser(strings.NewReader(`{}`))), nil
				})}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.DoSigned(context.Background(), http.MethodPost, "/auth", []byte(`{}`))
			if err == nil || calls != 1 || policyCalls != 1 {
				t.Fatalf("calls=%d policyCalls=%d err=%v; custom policy must stop after one attempt", calls, policyCalls, err)
			}
		})
	}
}

type trackedResponseBody struct {
	read   func([]byte) (int, error)
	closed bool
}

func (b *trackedResponseBody) Read(p []byte) (int, error) { return b.read(p) }
func (b *trackedResponseBody) Close() error               { b.closed = true; return nil }

func TestResponseReadRetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		mode      string
		attempts  int
		wantErr   error
		temporary bool
	}{
		{name: "unexpected EOF", status: 200, attempts: 2},
		{name: "server error with truncated body", status: 503, attempts: 2},
		{name: "attempt timeout", status: 200, mode: "timeout", attempts: 2},
		{name: "retries exhausted", status: 200, mode: "always fail", attempts: 3, wantErr: io.ErrUnexpectedEOF, temporary: true},
		{name: "policy rejects", status: 200, mode: "reject", attempts: 1, wantErr: io.ErrUnexpectedEOF},
		{name: "body exceeds cap", status: 200, mode: "oversize", attempts: 1, wantErr: errResponseTooLarge},
		{name: "unauthorized truncated body", status: 401, attempts: 1, wantErr: io.ErrUnexpectedEOF},
		{name: "parent canceled", status: 200, mode: "cancel", attempts: 1, wantErr: context.Canceled},
		{name: "parent deadline", status: 200, mode: "parent deadline", attempts: 1, wantErr: context.DeadlineExceeded},
	} {
		for _, method := range []string{http.MethodPost, http.MethodGet} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tc.mode == "parent deadline" {
					var deadlineCancel context.CancelFunc
					ctx, deadlineCancel = context.WithTimeout(ctx, 30*time.Millisecond)
					defer deadlineCancel()
				}
				policy := transport.RetryPolicy{MaxAttempts: 3}
				if tc.mode == "reject" {
					policy.RetryOnTransportError = func(error) bool { return false }
				}
				calls := 0
				signer := &countingSigner{}
				var bodies []*trackedResponseBody
				var contexts []context.Context
				var firstBody, firstURL, firstAuth string
				rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					var body []byte
					if r.Body != nil {
						var err error
						body, err = io.ReadAll(r.Body)
						if err != nil {
							t.Fatal(err)
						}
						_ = r.Body.Close()
					}
					if calls == 1 {
						firstBody, firstURL, firstAuth = string(body), r.URL.String(), r.Header.Get("Authorization")
					} else {
						if string(body) != firstBody || r.URL.String() != firstURL || r.Header.Get("Authorization") != firstAuth {
							t.Fatal("retry changed signed request bytes or authorization")
						}
						if contexts[len(contexts)-1].Err() == nil || !bodies[len(bodies)-1].closed {
							t.Fatal("previous attempt context/body was not released before retry")
						}
					}
					contexts = append(contexts, r.Context())
					bodyReader := &trackedResponseBody{}
					bodies = append(bodies, bodyReader)
					status := http.StatusOK
					if calls == 1 || tc.mode == "always fail" {
						status = tc.status
						bodyReader.read = func([]byte) (int, error) {
							switch tc.mode {
							case "timeout", "parent deadline":
								<-r.Context().Done()
								return 0, r.Context().Err()
							case "cancel":
								cancel()
								return 0, context.Canceled
							default:
								return 0, io.ErrUnexpectedEOF
							}
						}
						if tc.mode == "oversize" {
							bodyReader.read = strings.NewReader(strings.Repeat("x", 65)).Read
						}
					} else {
						bodyReader.read = strings.NewReader(`{"code":"SUCCESS"}`).Read
					}
					return testHTTPResponse(status, bodyReader), nil
				})
				timeout := time.Second
				if tc.mode == "timeout" {
					timeout = 30 * time.Millisecond
				}
				c, err := New(WithSigner(signer), WithBaseURL("https://test.invalid"), WithRetry(policy),
					WithHTTPClient(&http.Client{Transport: rt}), WithTimeout(timeout), WithMaxResponseBytes(64))
				if err != nil {
					t.Fatal(err)
				}
				var resp *RawResponse
				if method == http.MethodPost {
					resp, err = c.DoSigned(ctx, method, "/auth", []byte(`{"requestId":"stable-id"}`))
				} else {
					resp, err = c.DoSignedGET(ctx, "/query-auth", url.Values{"requestId": {"stable-id"}})
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err=%v, want %v", err, tc.wantErr)
				}
				if tc.wantErr == nil && (resp == nil || string(resp.Body) != `{"code":"SUCCESS"}`) {
					t.Fatalf("response=%+v; want successful response", resp)
				}
				var te *TransportError
				if err != nil && (!errors.As(err, &te) || te.Temporary() != tc.temporary) {
					t.Errorf("err=%v; want TransportError with Temporary=%v", err, tc.temporary)
				}
				if calls != tc.attempts || signer.calls != 1 {
					t.Errorf("attempts=%d sign calls=%d; want %d attempts, one signature", calls, signer.calls, tc.attempts)
				}
				for i, b := range bodies {
					if !b.closed || contexts[i].Err() == nil {
						t.Errorf("attempt %d leaked body or context", i+1)
					}
				}
			})
		}
	}
}
