package atomefin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

func TestWithKeyIDOptionOrder(t *testing.T) {
	key := mustGenKey(t)
	keyPEM := mustPEM(t, key)
	custom, err := sign.NewRSA2Signer(key, sign.WithKeyID("signer-default"))
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"requestId":"key-rotation"}`)
	sig, err := custom.Sign(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts []Option
		want string
	}{
		{"before PEM", []Option{WithKeyID("v2"), WithPrivateKeyPEM(keyPEM)}, "v2"},
		{"after PEM", []Option{WithPrivateKeyPEM(keyPEM), WithKeyID("v2")}, "v2"},
		{"before custom signer", []Option{WithKeyID("v2"), WithSigner(custom)}, "v2"},
		{"after custom signer", []Option{WithSigner(custom), WithKeyID("v2")}, "v2"},
		{"preserve custom default", []Option{WithSigner(custom)}, "signer-default"},
		{"last override wins", []Option{WithKeyID("v1"), WithPrivateKeyPEM(keyPEM), WithKeyID("v2")}, "v2"},
		{"clear PEM key ID", []Option{WithKeyID("v1"), WithPrivateKeyPEM(keyPEM), WithKeyID("")}, ""},
		{"clear custom key ID", []Option{WithSigner(custom), WithKeyID("")}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var header string
			opts := append([]Option{}, tc.opts...)
			opts = append(opts, WithBaseURL("https://test.invalid"), WithAuthorizationScheme(SchemeAtomeKeyed),
				WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					header = r.Header.Get("Authorization")
					return testHTTPResponse(http.StatusOK, io.NopCloser(strings.NewReader(`{}`))), nil
				})}))
			c, err := New(opts...)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.DoSigned(context.Background(), http.MethodPost, "/auth", body); err != nil {
				t.Fatal(err)
			}
			if header != SchemeAtomeKeyed(sig, tc.want) {
				t.Errorf("Authorization does not carry the expected key ID %q and signature", tc.want)
			}
			if custom.KeyID() != "signer-default" {
				t.Error("WithKeyID mutated the caller-owned signer")
			}
		})
	}
}
