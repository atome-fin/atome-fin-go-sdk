package sign_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// External-vector tests prove the SDK's RSA2 implementation matches an
// independent reference (openssl), not just round-trips against itself.
// Without these, a bug that flipped from PKCS#1 v1.5 to PSS, or
// truncated the digest, or rebuilt the canonical bytes incorrectly,
// would silently pass the existing self-round-trip suite.
//
// Vector files in testdata/external_*:
//   - external_priv.pem  RSA-2048 private key (openssl-generated)
//   - external_pub.pem   matching public key
//   - external_body.json fixed signing-canonical bytes
//   - external_sig.b64   base64-standard openssl signature over the body
//
// See testdata/README.md for the regen command + algorithm fingerprint.

const (
	externalPrivPath = "testdata/external_priv.pem"
	externalPubPath  = "testdata/external_pub.pem"
	externalBodyPath = "testdata/external_body.json"
	externalSigPath  = "testdata/external_sig.b64"
)

func mustRead(t testing.TB, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func mustExternalSig(t testing.TB) string {
	t.Helper()
	// openssl base64 wraps; we strip whitespace so the on-disk format
	// can be either wrapped or single-line and the test stays robust.
	return strings.TrimSpace(string(mustRead(t, externalSigPath)))
}

func mustExternalBody(t testing.TB) []byte {
	t.Helper()
	// Body is signed BYTE-FOR-BYTE — do NOT trim or re-encode.
	// `printf` was used (not `echo`) when the fixture was generated so
	// there is no trailing newline; we read it raw.
	return mustRead(t, externalBodyPath)
}

func mustSignerFromExternalKey(t testing.TB) sign.Signer {
	t.Helper()
	priv, err := sign.LoadPrivateKeyPEM(mustRead(t, externalPrivPath))
	if err != nil {
		t.Fatalf("LoadPrivateKeyPEM: %v", err)
	}
	s, err := sign.NewRSA2Signer(priv)
	if err != nil {
		t.Fatalf("NewRSA2Signer: %v", err)
	}
	return s
}

func mustVerifierFromExternalCert(t testing.TB) sign.Verifier {
	t.Helper()
	pub, err := sign.LoadPublicCertPEM(mustRead(t, externalPubPath))
	if err != nil {
		t.Fatalf("LoadPublicCertPEM: %v", err)
	}
	v, err := sign.NewRSA2Verifier(pub)
	if err != nil {
		t.Fatalf("NewRSA2Verifier: %v", err)
	}
	return v
}

// TestExternalVector_VerifyWithGo proves the Go verifier ACCEPTS a
// signature produced by openssl-out-of-band over the same bytes. This
// is the canonical "we agree with the reference" assertion.
func TestExternalVector_VerifyWithGo(t *testing.T) {
	v := mustVerifierFromExternalCert(t)
	body := mustExternalBody(t)
	sig := mustExternalSig(t)

	if err := v.Verify(context.Background(), body, sig); err != nil {
		t.Fatalf("Go verifier rejected an openssl-produced PKCS#1 v1.5 / SHA-256 signature: %v\n"+
			"this means the SDK and openssl disagree on the algorithm — see testdata/README.md", err)
	}
}

// TestExternalVector_SignWithGoMatchesOpenssl proves the Go signer
// produces BYTE-IDENTICAL output to openssl over the same private key
// + body. PKCS#1 v1.5 over SHA-256 is deterministic — there is no
// random salt, so the signature bytes are a function of (key, body).
//
// If this test ever fails, either:
//   - the SDK switched to PSS (non-deterministic — see PSSDoesNotMatchPKCS1v15)
//   - the SDK changed the hash (SHA-256 → SHA-512, etc.)
//   - the SDK changed the canonical input (e.g., normalised whitespace)
//
// Each of those is a wire-incompatible regression.
func TestExternalVector_SignWithGoMatchesOpenssl(t *testing.T) {
	signer := mustSignerFromExternalKey(t)
	body := mustExternalBody(t)
	wantSig := mustExternalSig(t)

	gotSig, err := signer.Sign(context.Background(), body)
	if err != nil {
		t.Fatalf("Go Sign: %v", err)
	}

	if gotSig != wantSig {
		// Show the first differing byte so a regression is debuggable
		// without the test author having to re-run openssl by hand.
		gotBytes, _ := base64.StdEncoding.DecodeString(gotSig)
		wantBytes, _ := base64.StdEncoding.DecodeString(wantSig)
		var firstDiff int = -1
		for i := 0; i < len(gotBytes) && i < len(wantBytes); i++ {
			if gotBytes[i] != wantBytes[i] {
				firstDiff = i
				break
			}
		}
		t.Fatalf("Go signature does NOT match openssl reference vector\n"+
			"first differing byte: %d (of %d)\n"+
			"go:      %s\nopenssl: %s",
			firstDiff, len(wantBytes), gotSig, wantSig)
	}
}

// TestExternalVector_TamperedBodyFails proves verification rejects when
// even one byte of the body is altered. A passive attacker who can
// observe a signed callback must not be able to swap the body
// underneath it.
func TestExternalVector_TamperedBodyFails(t *testing.T) {
	v := mustVerifierFromExternalCert(t)
	body := mustExternalBody(t)
	sig := mustExternalSig(t)

	// Flip the last byte of the body. Any single bit-flip suffices —
	// SHA-256 propagates the change across the whole digest.
	tampered := append([]byte(nil), body...)
	tampered[len(tampered)-1] ^= 0x01

	err := v.Verify(context.Background(), tampered, sig)
	if err == nil {
		t.Fatal("verifier accepted a tampered body — algorithm is broken")
	}
	if !errors.Is(err, sign.ErrSignature) {
		t.Errorf("err = %v; want sign.ErrSignature", err)
	}
}

// TestExternalVector_CanonicalQueryRoundTrip proves the GET-path
// canonical (`sign.CanonicalQuery`) is byte-compatible with an
// openssl signature over the same canonical string. The five v1 spec
// endpoints are all POST, but the spec reserves a GET signing path:
//
//	"GET: Sign the request parameters which parameter names are
//	 sorted in alphabetical natural order."
//
// This test is the forward-compat anchor: when a future spec
// revision adds a GET endpoint, the SDK's CanonicalQuery output
// already matches what an openssl reference would sign.
func TestExternalVector_CanonicalQueryRoundTrip(t *testing.T) {
	// Inputs were chosen so the alphabetical-sort produces:
	//   externalReferenceUid=user-42&periodType=3&requestId=01HABC...&totalAmount=1500000
	// (committed verbatim under testdata/external_query_canonical.txt).
	values := url.Values{
		"requestId":            []string{"01HABC1234567890ABCDEFGHJK"},
		"externalReferenceUid": []string{"user-42"},
		"totalAmount":          []string{"1500000"},
		"periodType":           []string{"3"},
	}
	canonical, err := sign.CanonicalQuery(values)
	if err != nil {
		t.Fatalf("CanonicalQuery: %v", err)
	}

	wantCanonical := strings.TrimSpace(string(mustRead(t, "testdata/external_query_canonical.txt")))
	if canonical != wantCanonical {
		t.Fatalf("CanonicalQuery output drifted from openssl-signed canonical\n"+
			"got:  %s\nwant: %s", canonical, wantCanonical)
	}

	// Verify the openssl-produced signature against the SDK's canonical bytes.
	v := mustVerifierFromExternalCert(t)
	wantSig := strings.TrimSpace(string(mustRead(t, "testdata/external_query_sig.b64")))
	if vErr := v.Verify(context.Background(), []byte(canonical), wantSig); vErr != nil {
		t.Fatalf("openssl signature over the canonical query did not verify against "+
			"sign.CanonicalQuery output: %v\nthis means the GET signing path is "+
			"broken — see DESIGN.md §1.3 / §4 / §5", vErr)
	}

	// And: Go signer over the same key + canonical produces the
	// identical base64 (PKCS#1 v1.5 deterministic).
	signer := mustSignerFromExternalKey(t)
	gotSig, err := signer.Sign(context.Background(), []byte(canonical))
	if err != nil {
		t.Fatal(err)
	}
	if gotSig != wantSig {
		t.Errorf("Go signer over CanonicalQuery output != openssl reference\ngo:      %s\nopenssl: %s",
			gotSig, wantSig)
	}
}

// TestExternalVector_PSSDoesNotMatchPKCS1v15 was removed in v0.7.0
// along with the PSS scaffolding. The openssl-anchored vector test
// above is the live byte-equality pin against PKCS#1 v1.5; PSS is
// no longer a code path — see CHANGELOG `## [0.7.0]`.
