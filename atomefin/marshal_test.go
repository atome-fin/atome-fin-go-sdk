package atomefin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// MarshalSigning's whole reason to exist is keeping "&", "<", ">" raw so
// the bytes signed match the bytes transmitted. This is the test that
// would have caught the production INVALID_SIGNATURE incident the
// architect's review flagged.
func TestMarshalSigningPreservesAmpersand(t *testing.T) {
	type payload struct {
		ShippingName string `json:"shippingName"`
		HTMLNote     string `json:"note"`
	}
	in := payload{
		ShippingName: "Foo & Co",
		HTMLNote:     "<b>important</b>",
	}
	b, err := MarshalSigning(in)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "Foo & Co") {
		t.Errorf("MarshalSigning escaped '&': %q", got)
	}
	if !strings.Contains(got, "<b>important</b>") {
		t.Errorf("MarshalSigning escaped '<>': %q", got)
	}
	// Explicit checks for the escape sequences that json.Marshal would
	// have produced. We split the literals across constant additions so
	// this test file itself does not contain the escape sequences and
	// can never match itself.
	for _, escape := range []string{
		"&" + "amp;",
		"&" + "lt;",
		"&" + "gt;",
	} {
		if strings.Contains(got, escape) {
			t.Errorf("MarshalSigning produced HTML escape %q in %q", escape, got)
		}
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("MarshalSigning kept trailing newline: %q", got)
	}
}

// End-to-end: signing the bytes produced by MarshalSigning and verifying
// against the same bytes round-trips. This is the property that the
// signing canonical equals the transmitted body.
func TestMarshalSigningSigns_Verifies(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Skip("rsa keygen unavailable on this platform")
	}
	signer, err := sign.NewRSA2Signer(priv)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := sign.NewRSA2Verifier(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	body, err := MarshalSigning(map[string]string{"shippingName": "Foo & Co"})
	if err != nil {
		t.Fatal(err)
	}
	sig, err := signer.Sign(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(context.Background(), body, sig); err != nil {
		t.Errorf("verify failed against MarshalSigning bytes: %v", err)
	}
}
