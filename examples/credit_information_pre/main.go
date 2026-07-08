// Command credit_information_pre exercises POST /credit-information against
// the pre-production gateway. It validates, in order:
//
//  1. Partner signing key + Atome encrypt public key load correctly
//  2. Local hybrid-encrypt + RSA2 sign pipeline (no network)
//  3. Live connectivity to the pre env (skipped when ATOME_FIN_DRY_RUN=1)
//
// Environment variables:
//
//	ATOME_FIN_PRIV_KEY_PEM              path to partner RSA-2048 signing private key (PEM)
//	ATOME_FIN_ATOME_ENCRYPT_CERT_PEM    path to Atome RSA-2048 encrypt public key (PEM)
//	ATOME_FIN_EXTERNAL_UID              partner user id (default: grab-pre-<unix>)
//	ATOME_FIN_MOBILE_NUMBER             e.g. +628129801929
//	ATOME_FIN_EMAIL                     user email
//	ATOME_FIN_FULL_NAME                 applicationEssentialInfo.ocrResult.fullName
//	ATOME_FIN_ENV                       pre|prod (default: pre)
//	ATOME_FIN_BASE_URL                  explicit base URL — overrides ATOME_FIN_ENV
//	ATOME_FIN_DRY_RUN                   "1" to stop after local crypto checks
//	ATOME_FIN_DEBUG                     "1" to enable SDK request/response body logging
//	ATOME_FIN_SKIP_CURL                 "1" to suppress the printable curl command
//
// Build & run:
//
//	go build ./examples/credit_information_pre/
//	ATOME_FIN_PRIV_KEY_PEM=/path/to/partner_sign_priv.pem \
//	ATOME_FIN_ATOME_ENCRYPT_CERT_PEM=/path/to/atome_encrypt_pub.pem \
//	./credit_information_pre
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

func main() {
	log.SetFlags(0)

	privPEM, err := readPEMFromPath(mustEnv("ATOME_FIN_PRIV_KEY_PEM"))
	if err != nil {
		log.Fatalf("partner signing private key: %v", err)
	}
	encryptPubPEM, err := readPEMFromPath(mustEnv("ATOME_FIN_ATOME_ENCRYPT_CERT_PEM"))
	if err != nil {
		log.Fatalf("atome encrypt public key: %v", err)
	}

	req := buildRequest()
	fmt.Println("==== credit-information pre check ====")
	fmt.Printf("externalReferenceUid : %s\n", req.ExternalReferenceUID)
	fmt.Printf("requestId            : %s\n", req.RequestID)
	fmt.Printf("mobileNumber         : %s\n", req.MobileNumber)
	fmt.Printf("email                : %s\n", req.Email)
	fmt.Printf("fullName             : %s\n", req.ApplicationEssentialInfo.OCRResult.FullName)
	fmt.Println()

	baseURL, err := resolveBaseURL()
	if err != nil {
		log.Fatalf("base url: %v", err)
	}
	fmt.Printf("target baseURL       : %s\n", baseURL)
	fmt.Println()

	wire, err := buildWireRequest(req, privPEM, encryptPubPEM, baseURL)
	if err != nil {
		log.Fatalf("local crypto check FAILED: %v", err)
	}
	fmt.Println("local crypto check   : PASS (marshal + hybrid-encrypt + RSA2 sign)")

	if os.Getenv("ATOME_FIN_SKIP_CURL") != "1" {
		printCurl(wire)
	}

	if os.Getenv("ATOME_FIN_DRY_RUN") == "1" {
		fmt.Println("connectivity         : SKIPPED (ATOME_FIN_DRY_RUN=1)")
		fmt.Println()
		fmt.Println("all checks passed (dry-run)")
		return
	}

	resp, err := dispatchWireRequest(wire)
	if err != nil {
		reportLiveFailure(err)
		os.Exit(1)
	}

	fmt.Println("connectivity         : PASS")
	fmt.Println()
	fmt.Println("---- /credit-information response ----")
	fmt.Printf("code    : %s\n", resp.Code)
	fmt.Printf("message : %s\n", resp.Message)
	if resp.Data != nil {
		fmt.Printf("requestId : %s\n", resp.Data.RequestID)
		fmt.Printf("status    : %s\n", resp.Data.Status)
		if resp.Data.JumpURL != "" {
			fmt.Printf("jumpUrl   : %s\n", resp.Data.JumpURL)
		}
	}
	fmt.Println()
	fmt.Println("all checks passed")
}

func buildRequest() *credit.CreditInformationParam {
	uid := envOr("ATOME_FIN_EXTERNAL_UID", fmt.Sprintf("grab-pre-%d", time.Now().Unix()))
	return &credit.CreditInformationParam{
		RequestID:            envOr("ATOME_FIN_REQUEST_ID", atomefin.DefaultRequestID()),
		ExternalReferenceUID: uid,
		MobileNumber:         envOr("ATOME_FIN_MOBILE_NUMBER", "+628129801929"),
		Email:                envOr("ATOME_FIN_EMAIL", "grab-pre-test@example.com"),
		Country:              credit.CountryIndonesia,
		ApplicationEssentialInfo: &credit.CreditInformationEssentialInfo{
			OCRResult: &credit.CreditInformationOCRResult{
				FullName: envOr("ATOME_FIN_FULL_NAME", "Grab Pre Test"),
			},
		},
		ExtendInfo: &credit.CreditInformationExtendInfo{
			Language: credit.LanguageIndonesian,
		},
	}
}

type wireRequest struct {
	URL           string
	PlainJSON     string
	EncryptedBody string
	EncryptHeader string
	Authorization string
	UserAgent     string
}

func buildWireRequest(req *credit.CreditInformationParam, privPEM, encryptPubPEM []byte, baseURL string) (*wireRequest, error) {
	plain, err := atomefin.MarshalSigning(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	if !json.Valid(plain) {
		return nil, errors.New("marshal request: output is not valid JSON")
	}

	priv, err := sign.LoadPrivateKeyPEM(privPEM)
	if err != nil {
		return nil, fmt.Errorf("load partner signing private key: %w", err)
	}
	atomeEncryptPub, err := sign.LoadPublicCertPEM(encryptPubPEM)
	if err != nil {
		return nil, fmt.Errorf("load atome encrypt public key: %w", err)
	}

	encHeader, bodyB64, err := encrypt.Marshal(plain, atomeEncryptPub)
	if err != nil {
		return nil, fmt.Errorf("hybrid encrypt: %w", err)
	}
	if !strings.HasPrefix(encHeader, "symmetricKey=") {
		return nil, fmt.Errorf("encrypt header malformed: %q", encHeader)
	}
	if strings.Contains(bodyB64, `"mobileNumber"`) {
		return nil, errors.New("encrypted body still contains plaintext field mobileNumber")
	}

	signer, err := sign.NewRSA2Signer(priv)
	if err != nil {
		return nil, fmt.Errorf("build signer: %w", err)
	}
	sig, err := signer.Sign(context.Background(), []byte(bodyB64))
	if err != nil {
		return nil, fmt.Errorf("sign encrypted body: %w", err)
	}
	if sig == "" {
		return nil, errors.New("sign encrypted body: empty signature")
	}

	verifier, err := sign.NewRSA2Verifier(&priv.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("build verifier: %w", err)
	}
	if err := verifier.Verify(context.Background(), []byte(bodyB64), sig); err != nil {
		return nil, fmt.Errorf("verify signature round-trip: %w", err)
	}

	return &wireRequest{
		URL:           strings.TrimRight(baseURL, "/") + "/credit-information",
		PlainJSON:     string(plain),
		EncryptedBody: bodyB64,
		EncryptHeader: encHeader,
		Authorization: atomefin.SchemeRawBase64(sig, ""),
		UserAgent:     transport.BuildUserAgent(atomefin.SDKVersion, ""),
	}, nil
}

func printCurl(w *wireRequest) {
	fmt.Println("---- curl (copy/paste) ----")
	fmt.Println("# plaintext JSON (signed input is the AES-encrypted body below, not this):")
	fmt.Println("# " + w.PlainJSON)
	fmt.Println("curl --request POST \\")
	fmt.Printf("  --url %s \\\n", shellQuote(w.URL))
	fmt.Printf("  --header %s \\\n", shellQuote("Content-Type: application/json"))
	fmt.Printf("  --header %s \\\n", shellQuote("User-Agent: "+w.UserAgent))
	fmt.Printf("  --header %s \\\n", shellQuote("Encrypt: "+w.EncryptHeader))
	fmt.Printf("  --header %s \\\n", shellQuote("Authorization: "+w.Authorization))
	fmt.Printf("  --data-raw %s\n", shellQuote(w.EncryptedBody))
	fmt.Println()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func resolveBaseURL() (string, error) {
	if base := os.Getenv("ATOME_FIN_BASE_URL"); base != "" {
		return strings.TrimRight(base, "/"), nil
	}
	env := atomefin.EnvPre
	switch os.Getenv("ATOME_FIN_ENV") {
	case "prod":
		env = atomefin.EnvProd
	case "pre", "":
		env = atomefin.EnvPre
	default:
		return "", fmt.Errorf("ATOME_FIN_ENV=%q: want pre or prod", os.Getenv("ATOME_FIN_ENV"))
	}
	return atomefin.BaseURL(env)
}

func dispatchWireRequest(wire *wireRequest) (*credit.CreditInformationResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wire.URL, strings.NewReader(wire.EncryptedBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", wire.UserAgent)
	req.Header.Set("Encrypt", wire.EncryptHeader)
	req.Header.Set("Authorization", wire.Authorization)

	client := &http.Client{Timeout: 45 * time.Second}
	httpResp, err := client.Do(req)
	if err != nil {
		return nil, &atomefin.TransportError{Op: "http", URL: wire.URL, Err: err}
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, &atomefin.TransportError{Op: "read", URL: wire.URL, Err: err}
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		apiErr := &atomefin.APIError{
			HTTPStatus: httpResp.StatusCode,
			Endpoint:   "/credit-information",
			Raw:        body,
		}
		var env struct {
			Code    atomefin.Code `json:"code"`
			Message string        `json:"message"`
		}
		if err := json.Unmarshal(body, &env); err == nil {
			apiErr.Code = env.Code
			apiErr.Message = env.Message
		}
		return nil, apiErr
	}

	var out credit.CreditInformationResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: wire.URL,
			Err: fmt.Errorf("decode response: %w", err),
		}
	}
	return &out, nil
}

func reportLiveFailure(err error) {
	fmt.Println("connectivity         : FAIL")
	fmt.Println()

	var ve *atomefin.ValidationError
	var se *atomefin.SignatureError
	var te *atomefin.TransportError
	var ae *atomefin.APIError

	switch {
	case errors.As(err, &ve):
		fmt.Printf("category : local validation\n")
		fmt.Printf("field    : %s\n", ve.Field)
		fmt.Printf("detail   : %s\n", ve.Message)
		if ve.Field == "encryptAtomePublicCert" {
			fmt.Println("hint     : set ATOME_FIN_ATOME_ENCRYPT_CERT_PEM to Atome encrypt public key (E4), not signing public key")
		}
	case errors.As(err, &se):
		fmt.Printf("category : signing\n")
		fmt.Printf("reason   : %s\n", se.Reason)
		fmt.Printf("detail   : %v\n", se.Err)
		fmt.Println("hint     : confirm ATOME_FIN_PRIV_KEY_PEM is the partner signing private key registered with Atome")
	case errors.As(err, &te):
		fmt.Printf("category : transport/encrypt\n")
		fmt.Printf("op       : %s\n", te.Op)
		fmt.Printf("detail   : %v\n", te.Err)
		if te.Op == "encrypt" {
			fmt.Println("hint     : check ATOME_FIN_ATOME_ENCRYPT_CERT_PEM format (CERTIFICATE / PUBLIC KEY) and RSA >= 2048 bits")
		} else {
			fmt.Println("hint     : check network, DNS, TLS, and pre env base URL")
		}
	case errors.As(err, &ae):
		fmt.Printf("category : api error\n")
		fmt.Printf("http     : %d\n", ae.HTTPStatus)
		fmt.Printf("code     : %s\n", ae.Code)
		fmt.Printf("message  : %s\n", ae.Message)
		if ae.IsSignature() {
			fmt.Println("hint     : server rejected Authorization — partner signing public key may not be registered on pre")
		}
		switch ae.Code {
		case atomefin.CodeInvalidSignature:
			fmt.Println("hint     : verify partner signing key pair and that Authorization signs the encrypted body")
		case "INVALID_ENCRYPTION":
			fmt.Println("hint     : verify Atome encrypt public key (E4) matches the pre environment")
		case credit.CodeActiveAccount:
			fmt.Println("hint     : user already has an active account; try a fresh ATOME_FIN_EXTERNAL_UID")
		case credit.CodeCreditApplicationInProgress:
			fmt.Println("hint     : application already in progress for this user")
		}
	default:
		fmt.Printf("detail   : %v\n", err)
	}
}

func readPEMFromPath(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%q is empty", path)
	}
	return data, nil
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is empty", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
