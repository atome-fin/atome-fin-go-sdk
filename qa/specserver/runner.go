package specserver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// Case is one row in the per-package spec-assertion table. The Run
// closure takes a spec-server-pointing Client and invokes ONE SDK
// method, returning its error.
type Case struct {
	// Op is "METHOD /path" — must match a key in the loaded spec.
	// E.g. "POST /auth", "GET /query-refund".
	Op string

	// Run invokes the SDK method under test against the given
	// Client. The Client is pre-configured to talk to the spec
	// server. If Run returns a non-nil error AND the spec server
	// recorded no validation failure for this op, RunCases reports
	// the error as a regular test failure. If the spec server
	// recorded a validation failure (e.g. PARAMS_MISSING),
	// RunCases reports the validation diagnostic instead — the
	// SDK's error is the symptom, the missing field is the cause.
	Run func(*atomefin.Client) error

	// Fixture optionally points the spec server at a canned 200
	// response body for this op (path relative to module root).
	// Leave empty to use the default {"code":"SUCCESS"} envelope.
	Fixture string

	// SkipRequired is a per-case allowlist of spec-required field
	// paths (body or query) the SDK is knowingly not yet emitting
	// for this op. Each entry is forwarded to Server.SkipRequired
	// before the case runs. Use sparingly and document the reason
	// in a comment alongside the literal — every skip is a known
	// gap that should be tracked toward closure.
	SkipRequired []string
}

// RunCases drives every case against the package's pinned spec
// server. Failures are reported via t.Errorf with the structured
// diagnostic (op, missing field, request body / query for context,
// and the spec snapshot path).
//
// One server is shared across all cases — the framework's
// invariants are stateless modulo the per-case fixture override, so
// fan-out is safe. Per-case validation hits are isolated by the
// op-keyed dispatch map.
func RunCases(t *testing.T, cases []Case) {
	t.Helper()
	srv := New(t)

	// Load any per-case fixtures + register skip-allowlists up-front
	// so a missing fixture / typo fails the test deterministically
	// rather than surprising one case mid-run.
	for _, c := range cases {
		if c.Fixture != "" {
			if err := srv.SetFixture(c.Op, c.Fixture); err != nil {
				t.Fatalf("RunCases: %v", err)
			}
		}
		if len(c.SkipRequired) > 0 {
			srv.SkipRequired(c.Op, c.SkipRequired...)
		}
	}

	client := MustClient(t, srv)

	for _, c := range cases {
		c := c
		t.Run(sanitizeName(c.Op), func(t *testing.T) {
			before := len(srv.Failures())
			err := c.Run(client)
			after := srv.Failures()
			newFailures := after[before:]

			if len(newFailures) > 0 {
				for _, f := range newFailures {
					t.Errorf("%s", formatFailure(f))
				}
				return
			}
			if err != nil {
				t.Errorf("%s: SDK call returned error: %v", c.Op, err)
				return
			}
			if srv.Hits(c.Op) == 0 {
				t.Errorf("%s: SDK did not reach the spec server (hit count = 0)", c.Op)
			}
		})
	}
}

// MustClient builds an atomefin.Client wired to the spec server
// with a freshly-generated test keypair AND a freshly-generated
// encrypt keypair. The signing keypair signs outbound requests;
// the encrypt keypair satisfies the WithEncryptAtomePublicCertPEM
// precondition for endpoints that route through DoEncryptedSigned
// (today /credit-information, /credit-application). The spec
// server itself ignores signatures and ignores the encrypted body
// shape — payload validation against the pinned spec is the only
// concern — so the test keypair material is fine to be ephemeral.
func MustClient(t testing.TB, srv *Server) *atomefin.Client {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("specserver.MustClient: rsa.GenerateKey signing: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("specserver.MustClient: marshal signing public key: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	encKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("specserver.MustClient: rsa.GenerateKey encrypt: %v", err)
	}
	encPubDER, err := x509.MarshalPKIXPublicKey(&encKey.PublicKey)
	if err != nil {
		t.Fatalf("specserver.MustClient: marshal encrypt public key: %v", err)
	}
	encPubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: encPubDER})

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(privPEM),
		atomefin.WithAtomePublicCertPEM(pubPEM),
		atomefin.WithEncryptAtomePublicCertPEM(encPubPEM),
	)
	if err != nil {
		t.Fatalf("specserver.MustClient: atomefin.New: %v", err)
	}
	return c
}

// formatFailure renders a Failure into the architect-spec'd
// diagnostic format from SPEC_ASSERTION_TEST_DESIGN.md §1.2.
func formatFailure(f Failure) string {
	var b strings.Builder
	fmt.Fprintf(&b, "specserver: %s: %s", f.Op, f.Reason)
	if f.Field != "" {
		fmt.Fprintf(&b, " %q", f.Field)
	}
	if f.Body != "" {
		fmt.Fprintf(&b, "\n  request body: %s", f.Body)
	}
	if f.Query != "" {
		fmt.Fprintf(&b, "\n  wire query: %s", f.Query)
	}
	if f.SpecPath != "" {
		fmt.Fprintf(&b, "\n  spec: %s", f.SpecPath)
	}
	return b.String()
}

// sanitizeName turns "POST /auth" into "POST_auth" so go test's
// per-subtest naming is shell-friendly.
func sanitizeName(op string) string {
	r := strings.NewReplacer(" ", "_", "/", "_", "<", "", ">", "")
	return r.Replace(op)
}
