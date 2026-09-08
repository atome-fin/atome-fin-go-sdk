package virtualaccount

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// BankCode is a bank supported for Grab ID virtual-account repayment.
type BankCode string

// Spec-defined virtual-account bank codes.
const (
	BankBCA              BankCode = "BCA"
	BankMandiri          BankCode = "MANDIRI"
	BankBRI              BankCode = "BRI"
	BankBNI              BankCode = "BNI"
	BankPermata          BankCode = "PERMATA"
	BankSahabatSampoerna BankCode = "SAHABAT_SAMPOERNA"
)

// IsValid reports whether code is one of the spec-defined bank codes.
func (c BankCode) IsValid() bool {
	switch c {
	case BankBCA, BankMandiri, BankBRI, BankBNI, BankPermata, BankSahabatSampoerna:
		return true
	default:
		return false
	}
}

// String returns the wire literal verbatim.
func (c BankCode) String() string { return string(c) }

// VirtualAccountRequest is the POST /va/vaCodeByBank body.
type VirtualAccountRequest struct {
	// ExternalReferenceUID is the partner-side stable user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// BankCode is the selected virtual-account bank.
	BankCode BankCode `json:"bankCode"`
}

// VirtualAccount is the user's active or newly created VA.
type VirtualAccount struct {
	// VANumber is the virtual-account number for the selected bank.
	VANumber string `json:"vaNumber"`
	// BankCode is the selected virtual-account bank.
	BankCode BankCode `json:"bankCode"`
	// CreateTime is the VA creation time in Unix milliseconds.
	CreateTime int64 `json:"createTime"`
}

// VirtualAccountListResponse is the POST /va/getList envelope.
type VirtualAccountListResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
	Data    []BankCode    `json:"data"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *VirtualAccountListResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// VirtualAccountResponse is the POST /va/vaCodeByBank envelope.
type VirtualAccountResponse struct {
	Code    atomefin.Code   `json:"code"`
	Message string          `json:"message"`
	Data    *VirtualAccount `json:"data,omitempty"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *VirtualAccountResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// Service is the outbound virtual-account client. Construct with
// virtualaccount.New(c); it is immutable and safe for concurrent use.
type Service struct {
	c *atomefin.Client
}

// New returns a *Service bound to the given Client. It returns nil for
// a nil Client so calls fail fast with a typed validation error.
func New(c *atomefin.Client) *Service {
	if c == nil {
		return nil
	}
	return &Service{c: c}
}

// Client exposes the underlying *atomefin.Client. Nil-safe.
func (s *Service) Client() *atomefin.Client {
	if s == nil {
		return nil
	}
	return s.c
}

func (s *Service) checkConfigured() error {
	if s == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "nil *virtualaccount.Service (likely from virtualaccount.New(nil))",
		}
	}
	if s.c == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "*virtualaccount.Service has nil *atomefin.Client",
		}
	}
	return nil
}

// ListBanks submits POST /va/getList with the spec-required empty JSON
// object and returns the currently available VA bank codes.
func (s *Service) ListBanks(ctx context.Context) (*VirtualAccountListResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	body, err := atomefin.MarshalSigning(struct{}{})
	if err != nil {
		return nil, &atomefin.SignatureError{Reason: "marshal", Err: err}
	}
	resp, err := s.c.DoSigned(ctx, http.MethodPost, "/va/getList", body)
	if err != nil {
		return nil, err
	}
	var out VirtualAccountListResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/va/getList",
			Err: fmt.Errorf("decode /va/getList response: %w", err),
		}
	}
	return &out, nil
}

// GetOrCreate submits POST /va/vaCodeByBank. If the user has no active
// VA for the selected bank, Atome creates one and returns it.
func (s *Service) GetOrCreate(ctx context.Context, req *VirtualAccountRequest) (*VirtualAccountResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil VirtualAccountRequest"}
	}
	if err := validateVirtualAccountRequest(req); err != nil {
		return nil, err
	}
	body, err := atomefin.MarshalSigning(req)
	if err != nil {
		return nil, &atomefin.SignatureError{Reason: "marshal", Err: err}
	}
	resp, err := s.c.DoSigned(ctx, http.MethodPost, "/va/vaCodeByBank", body)
	if err != nil {
		return nil, err
	}
	var out VirtualAccountResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/va/vaCodeByBank",
			Err: fmt.Errorf("decode /va/vaCodeByBank response: %w", err),
		}
	}
	return &out, nil
}

func validateVirtualAccountRequest(req *VirtualAccountRequest) error {
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if len(req.ExternalReferenceUID) > 64 {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "exceeds spec maxlength 64"}
	}
	if !req.BankCode.IsValid() {
		return &atomefin.ValidationError{
			Field:   "bankCode",
			Message: "must be one of BCA | MANDIRI | BRI | BNI | PERMATA | SAHABAT_SAMPOERNA",
		}
	}
	return nil
}
