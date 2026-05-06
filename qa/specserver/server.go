package specserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// Server is the strict spec-validating HTTP test server.
// httptest.NewServer-compatible (Server.URL is the base URL the
// SDK Client should be pointed at). Construct via New(t).
type Server struct {
	*httptest.Server
	Spec *Spec

	mu       sync.Mutex
	fixtures map[string]json.RawMessage // op-key → response body
	hits     map[string]int             // op-key → invocation count
	failures []Failure                  // structured spec-validation failures
	skipReq  map[string]map[string]bool // op-key → set of required-paths to skip
}

// Failure is the structured record emitted when a request fails the
// spec-validation pass. RunCases surfaces these to t.Errorf with full
// diagnostic context; standalone callers can read them via
// Server.Failures.
type Failure struct {
	Op       string // "POST /auth"
	Reason   string // "missing required body field"
	Field    string // "subOrders[0].subOrderId"
	Body     string // truncated request body
	Query    string // raw query string for GETs
	SpecPath string // pinned spec absolute path
}

// New builds a Server backed by the package's pinned swagger.yaml
// (qa/specserver/testdata/swagger-*.yaml). Equivalent to NewWithSpec
// after a LoadDefault() call. The returned server is started and
// ready for requests; the caller must Close it (typically via
// t.Cleanup).
func New(t testing.TB) *Server {
	t.Helper()
	spec, err := LoadDefault()
	if err != nil {
		t.Fatalf("specserver.New: %v", err)
	}
	return NewWithSpec(t, spec)
}

// NewWithSpec is the testable seam — handy when a self-test wants to
// load a custom spec for negative-path coverage.
func NewWithSpec(t testing.TB, spec *Spec) *Server {
	t.Helper()
	s := &Server{
		Spec:     spec,
		fixtures: make(map[string]json.RawMessage),
		hits:     make(map[string]int),
		skipReq:  make(map[string]map[string]bool),
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Server.Close)
	return s
}

// SetFixture loads a JSON file as the canned 200 response body for
// op (e.g. SetFixture("POST /auth", "qa/testdata/auth_response_success.json")).
// If unset, the server emits a generic
// {"code":"SUCCESS","message":"ok"} envelope so the SDK doesn't
// fail decoding.
func (s *Server) SetFixture(op, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("specserver: load fixture %q: %w", path, err)
	}
	// Validate JSON — fail loud in test setup, not in the handler.
	if !json.Valid(data) {
		return fmt.Errorf("specserver: fixture %q is not valid JSON", path)
	}
	s.mu.Lock()
	s.fixtures[normalizeOpKey(op)] = json.RawMessage(data)
	s.mu.Unlock()
	return nil
}

// SetFixtureRaw sets an inline JSON fixture for op. Useful when a
// per-test happy-path body diverges from the shared fixture set.
func (s *Server) SetFixtureRaw(op string, body json.RawMessage) {
	s.mu.Lock()
	s.fixtures[normalizeOpKey(op)] = body
	s.mu.Unlock()
}

// SkipRequired registers a per-op allowlist of required field paths
// (body or query) that the spec server should skip when validating
// requests for that op. Use sparingly — it is a knowing
// acknowledgement that the SDK does not yet emit a spec-required
// field, typically because the field is partner-pending or the
// spec is "Initial draft" and the upstream gateway is known not to
// enforce it. Each call APPENDS to the existing skip set.
//
// Document the reason for each skip in the calling test alongside
// the SkipRequired call so the next reviewer sees the intent.
//
// Example:
//
//	srv.SkipRequired("POST /payment-plan",
//	    "subOrders[].categoryId",      // spec "Initial draft" — partner Q-set
//	    "subOrders[].merchantId",      // same
//	    "extendInfo.paymentType",      // not yet declared in v0.2 surface
//	)
func (s *Server) SkipRequired(op string, paths ...string) {
	key := normalizeOpKey(op)
	s.mu.Lock()
	defer s.mu.Unlock()
	set, ok := s.skipReq[key]
	if !ok {
		set = make(map[string]bool, len(paths))
		s.skipReq[key] = set
	}
	for _, p := range paths {
		set[p] = true
	}
}

// effectiveRequired returns the spec's RequiredBody / RequiredQuery
// for op with any registered skips removed. Used by the dispatcher.
func (s *Server) effectiveRequired(op Operation) (body, query []string) {
	key := opKey(op.Method, op.Path)
	s.mu.Lock()
	skip := s.skipReq[key]
	s.mu.Unlock()
	if len(skip) == 0 {
		return op.RequiredBody, op.RequiredQuery
	}
	body = make([]string, 0, len(op.RequiredBody))
	for _, p := range op.RequiredBody {
		if !skip[p] {
			body = append(body, p)
		}
	}
	query = make([]string, 0, len(op.RequiredQuery))
	for _, p := range op.RequiredQuery {
		if !skip[p] {
			query = append(query, p)
		}
	}
	return body, query
}

// Hits returns the invocation count for op, regardless of whether
// the request passed validation.
func (s *Server) Hits(op string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[normalizeOpKey(op)]
}

// normalizeOpKey turns a public-API "Method /path" string into the
// internal op-key shape (uppercase method, verbatim path). Robust
// against callers that pass either case.
func normalizeOpKey(op string) string {
	i := strings.IndexByte(op, ' ')
	if i < 0 {
		return op
	}
	return strings.ToUpper(op[:i]) + op[i:]
}

// Failures returns the structured validation-failure log captured so
// far. Order of appearance reflects request order.
func (s *Server) Failures() []Failure {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Failure, len(s.failures))
	copy(out, s.failures)
	return out
}

// Reset clears the hits and failures counters (fixtures untouched).
// Useful between sub-tests.
func (s *Server) Reset() {
	s.mu.Lock()
	s.hits = make(map[string]int)
	s.failures = nil
	s.mu.Unlock()
}

// handle is the http.HandlerFunc.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	op := opKey(r.Method, r.URL.Path)
	s.mu.Lock()
	s.hits[op]++
	s.mu.Unlock()

	// Look up the operation. Unknown → 404 PARAMS_WRONG (matches
	// the upstream gateway's behaviour).
	specOp, ok := s.Spec.Op(r.Method, r.URL.Path)
	if !ok {
		s.recordFailure(Failure{
			Op:       op,
			Reason:   "operation not declared in pinned spec",
			SpecPath: s.Spec.Path,
		})
		writeJSONError(w, http.StatusNotFound, "PARAMS_WRONG", fmt.Sprintf("operation %s not in pinned spec", op))
		return
	}

	requiredBody, requiredQuery := s.effectiveRequired(specOp)

	// GET: validate query params.
	if r.Method == http.MethodGet {
		gotQ := r.URL.Query()
		for _, name := range requiredQuery {
			if gotQ.Get(name) == "" {
				s.recordFailure(Failure{
					Op:       op,
					Reason:   "missing required query param",
					Field:    name,
					Query:    r.URL.RawQuery,
					SpecPath: s.Spec.Path,
				})
				writeJSONError(w, http.StatusBadRequest, "PARAMS_MISSING",
					fmt.Sprintf("missing required query param %q", name))
				return
			}
		}
		s.writeFixture(w, op)
		return
	}

	// POST: validate body.
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MiB cap
		_ = r.Body.Close()
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "WRONG_PARAMS_FORMAT", "read body: "+err.Error())
			return
		}
		if missing, ferr := validateBody(body, requiredBody); ferr != nil {
			s.recordFailure(Failure{
				Op:       op,
				Reason:   "request body is not valid JSON",
				Body:     truncate(string(body), 256),
				SpecPath: s.Spec.Path,
			})
			writeJSONError(w, http.StatusBadRequest, "WRONG_PARAMS_FORMAT", ferr.Error())
			return
		} else if missing != "" {
			s.recordFailure(Failure{
				Op:       op,
				Reason:   "missing required body field",
				Field:    missing,
				Body:     truncate(string(body), 256),
				SpecPath: s.Spec.Path,
			})
			writeJSONError(w, http.StatusBadRequest, "PARAMS_MISSING",
				fmt.Sprintf("missing required body field %q", missing))
			return
		}
		s.writeFixture(w, op)
		return
	}

	// Other verbs: out of scope.
	writeJSONError(w, http.StatusMethodNotAllowed, "PARAMS_WRONG", "verb not supported by spec server")
}

func (s *Server) recordFailure(f Failure) {
	s.mu.Lock()
	s.failures = append(s.failures, f)
	s.mu.Unlock()
}

func (s *Server) writeFixture(w http.ResponseWriter, op string) {
	s.mu.Lock()
	body, ok := s.fixtures[op]
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if ok {
		_, _ = w.Write(body)
		return
	}
	// Default minimal SUCCESS envelope. Avoids surprising the SDK's
	// response decoder when a per-op fixture isn't supplied.
	_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
