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

	specpkg "github.com/atome-fin/atome-fin-go-sdk/internal/spec"
)

// Spec / Operation are re-exported as the strict-mode SDK CI
// driver still talks in those terms. v0.5 promoted the underlying
// implementations to internal/spec; the surface every existing
// `*_spec_test.go` and `coverage_test.go` consumer reads from
// (`specserver.Case`, `specserver.RunCases`,
// `specserver.LoadDefault`, `specserver.PinnedPath`) is preserved
// verbatim.
type (
	// Spec is the parsed pinned swagger document.
	Spec = specpkg.Spec
	// Operation captures the per-endpoint required-field sets.
	Operation = specpkg.Operation
)

// LoadDefault re-exports the internal/spec default-loader for the
// pinned swagger snapshot.
func LoadDefault() (*Spec, error) { return specpkg.LoadDefault() }

// PinnedPath re-exports internal/spec.PinnedPath. Used by drift_test.go.
func PinnedPath() (string, error) { return specpkg.PinnedPath() }

// Server is the strict spec-validating HTTP test server. Same shape
// as v0.4 but its validation primitives now live in
// `internal/spec`.
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

// New builds a Server backed by the package's pinned swagger.yaml.
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

// SetFixture loads a JSON file as the canned 200 response body for op.
func (s *Server) SetFixture(op, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("specserver: load fixture %q: %w", path, err)
	}
	if !json.Valid(data) {
		return fmt.Errorf("specserver: fixture %q is not valid JSON", path)
	}
	s.mu.Lock()
	s.fixtures[normalizeOpKey(op)] = json.RawMessage(data)
	s.mu.Unlock()
	return nil
}

// SetFixtureRaw sets an inline JSON fixture for op.
func (s *Server) SetFixtureRaw(op string, body json.RawMessage) {
	s.mu.Lock()
	s.fixtures[normalizeOpKey(op)] = body
	s.mu.Unlock()
}

// SkipRequired registers a per-op allowlist of required field paths
// (body or query) that the spec server should skip when validating
// requests for that op.
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

// effectiveRequired returns the spec's RequiredBody / RequiredQuery /
// RequiredHeader for op with any registered skips removed.
func (s *Server) effectiveRequired(op Operation) (body, query, header []string) {
	key := specpkg.OpKey(op.Method, op.Path)
	s.mu.Lock()
	skip := s.skipReq[key]
	s.mu.Unlock()
	if len(skip) == 0 {
		return op.RequiredBody, op.RequiredQuery, op.RequiredHeader
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
	header = make([]string, 0, len(op.RequiredHeader))
	for _, p := range op.RequiredHeader {
		if !skip[p] {
			header = append(header, p)
		}
	}
	return body, query, header
}

// Hits returns the invocation count for op.
func (s *Server) Hits(op string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[normalizeOpKey(op)]
}

// normalizeOpKey turns a public-API "Method /path" string into the
// internal op-key shape (uppercase method, verbatim path).
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
func (s *Server) Reset() {
	s.mu.Lock()
	s.hits = make(map[string]int)
	s.failures = nil
	s.mu.Unlock()
}

// handle is the http.HandlerFunc.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	op := specpkg.OpKey(r.Method, r.URL.Path)
	s.mu.Lock()
	s.hits[op]++
	s.mu.Unlock()

	// Look up the operation. Unknown → 404 PARAMS_WRONG.
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

	requiredBody, requiredQuery, requiredHeader := s.effectiveRequired(specOp)

	// Header presence (applies to all verbs).
	encryptedRequest := false
	for _, name := range requiredHeader {
		if r.Header.Get(name) == "" {
			s.recordFailure(Failure{
				Op:       op,
				Reason:   "missing required header",
				Field:    name,
				SpecPath: s.Spec.Path,
			})
			writeJSONError(w, http.StatusBadRequest, "PARAMS_MISSING",
				fmt.Sprintf("missing required header %q", name))
			return
		}
		if name == "Encrypt" {
			encryptedRequest = true
		}
	}

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
		// Encrypted body: spec server has no decryption key.
		if encryptedRequest {
			s.writeFixture(w, op)
			return
		}
		if missing, ferr := specpkg.ValidateBody(body, requiredBody); ferr != nil {
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
