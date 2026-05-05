package sign

import "crypto/rsa"

// This file scaffolds the optional RSA-PSS ("salted") signing variant.
//
// Per the apaylater spec's Signature section (DESIGN.md §1.3 verbatim):
//
//	"Encrypt the signature with salt if necessary, and in this
//	 condition we should exchange another public key certificate."
//
// PSS is therefore the *conditional* path that runs alongside the
// default PKCS#1 v1.5 flow, and the spec mandates a SEPARATE cert
// exchange for it. The default (anchored by atomefin/sign/testdata/
// external_*) is PKCS#1 v1.5; PSS only fires when the partner has
// flipped it on after the cert exchange.
//
// # v0.1 limitation (Q2b — see DESIGN.md §13)
//
// Today's WithSaltedPSS / WithVerifierSaltedPSS options flip a single
// Signer / Verifier from PKCS#1 v1.5 to PSS but reuse whatever key
// was passed at construction time. Spec-compliant PSS deployment
// needs a SECOND keypair bound separately on the Client. v0.2 plan:
// add WithSaltedPSSPrivateKeyPEM / WithSaltedPSSAtomePublicCertPEM
// Options that hold PSS keys alongside the default-path keys.
//
// Documentation-only for v0.1 because the prod-default path
// (PKCS#1 v1.5, the openssl-vector path) is what every partner is
// expected to use today; PSS adopters in v0.1 use the same keypair
// for both modes and accept that the wire-format will diverge from
// a spec-compliant partner reference implementation.

// WithSaltedPSS enables RSA-PSS signing on the Signer. saltLen <= 0 selects
// rsa.PSSSaltLengthAuto.
//
// PSS is gated on a separate cert exchange — only flip this on once the
// partner confirms (DESIGN.md §10/Q4 + Q2b for the v0.2 separate-cert plan).
func WithSaltedPSS(saltLen int) SignerOption {
	return func(s *rsa2Signer) error {
		s.pss = true
		s.saltLen = normalizeSaltLen(saltLen)
		return nil
	}
}

// WithVerifierSaltedPSS enables RSA-PSS verification on the Verifier. The
// salt-length parameter mirrors WithSaltedPSS.
//
// In practice rsa.PSSSaltLengthAuto on the verify path tolerates any salt
// length the signer chose, so passing 0 is a safe default once PSS is
// negotiated end-to-end.
func WithVerifierSaltedPSS(saltLen int) VerifierOption {
	return func(v *rsa2Verifier) error {
		v.pss = true
		v.saltLen = normalizeSaltLen(saltLen)
		return nil
	}
}

func normalizeSaltLen(saltLen int) int {
	if saltLen <= 0 {
		return rsa.PSSSaltLengthAuto
	}
	return saltLen
}
