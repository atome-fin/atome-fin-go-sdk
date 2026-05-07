package mock

import (
	_ "embed"
)

// Bundled mock keypairs — generated once at design time and
// committed to `atomefin/mock/testdata/` so partner tests don't
// have to call `rsa.GenerateKey` on every run (saves ~50ms / call,
// adds up across CI). Off by default; opt-in via
// `WithMockKeysAllowed`.
//
// **These keys are public, committed, and ONLY for testing.**
// They are clearly labelled in their PEM headers; production
// systems must never accept them.

//go:embed testdata/mock_signing_priv.pem
var mockSigningPrivPEM []byte

//go:embed testdata/mock_signing_pub.pem
var mockSigningPubPEM []byte

//go:embed testdata/mock_encrypt_priv.pem
var mockEncryptPrivPEM []byte

//go:embed testdata/mock_encrypt_pub.pem
var mockEncryptPubPEM []byte

// MockSigningPrivKeyPEM returns the bundled signing PRIVATE key
// PEM bytes. Use only for tests.
func MockSigningPrivKeyPEM() []byte {
	out := make([]byte, len(mockSigningPrivPEM))
	copy(out, mockSigningPrivPEM)
	return out
}

// MockSigningPubCertPEM returns the bundled signing PUBLIC key
// PEM bytes (matches MockSigningPrivKeyPEM). Use only for tests.
func MockSigningPubCertPEM() []byte {
	out := make([]byte, len(mockSigningPubPEM))
	copy(out, mockSigningPubPEM)
	return out
}

// MockEncryptPrivKeyPEM returns the bundled encrypt PRIVATE key
// PEM bytes (the partner-side key). Use only for tests.
func MockEncryptPrivKeyPEM() []byte {
	out := make([]byte, len(mockEncryptPrivPEM))
	copy(out, mockEncryptPrivPEM)
	return out
}

// MockEncryptPubCertPEM returns the bundled encrypt PUBLIC key
// PEM bytes (the Atome-side key). Use only for tests.
func MockEncryptPubCertPEM() []byte {
	out := make([]byte, len(mockEncryptPubPEM))
	copy(out, mockEncryptPubPEM)
	return out
}
