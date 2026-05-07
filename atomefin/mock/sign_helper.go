package mock

import (
	"context"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// signBodyWithPEM is the shared sign-bytes-with-PEM-key core
// used by the Server's auto-callback dispatcher
// (server_dispatch.go's signCallback / signResponseBody) AND
// the Fire*Callback helpers (callback.go's fire). Returns the
// base64 signature suitable for an Authorization header.
//
// Consolidated in v0.6.0: the v0.5.0 CHANGELOG claimed
// fire() and signCallback() shared a signing core, but the
// PEM-load + signer-build + sign sequence was duplicated. This
// helper makes that claim true — both call sites consume the
// same three lines via this function.
//
// ctx threads cancellation into the signer (sign.NewRSA2Signer's
// Sign honours ctx.Err() at entry).
func signBodyWithPEM(ctx context.Context, body []byte, privPEM []byte) (string, error) {
	priv, err := sign.LoadPrivateKeyPEM(privPEM)
	if err != nil {
		return "", err
	}
	signer, err := sign.NewRSA2Signer(priv)
	if err != nil {
		return "", err
	}
	return signer.Sign(ctx, body)
}
