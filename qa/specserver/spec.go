// Package specserver provides a spec-driven test harness for the SDK
// outbound surface. It loads a pinned upstream swagger.yaml, extracts
// the per-endpoint required-field set, and stands up an httptest
// server that rejects requests that omit required fields with a
// 400 PARAMS_MISSING envelope.
//
// The framework's job is narrow on purpose. It catches "the SDK
// didn't send a spec-required field" — the bug class that produced
// the v0.2.0 §2.1 regression (four GET methods missing
// `externalReferenceUid`). It does NOT type-check, enum-check, or
// maxLength-check; see SPEC_ASSERTION_TEST_DESIGN.md §1.7 for the
// full coverage boundary and the architect's "what is not caught"
// table.
//
// Usage from a per-package test:
//
//	func TestSpec_PaymentEndpoints(t *testing.T) {
//	    cases := []specserver.Case{
//	        {Op: "POST /auth", Run: func(c *atomefin.Client) error {
//	            _, err := payment.New(c).Auth(ctx, sampleAuthRequest())
//	            return err
//	        }},
//	        // …one row per endpoint
//	    }
//	    specserver.RunCases(t, cases)
//	}
//
// The server is hermetic and offline. The optional drift-detection
// test (TestSpec_PinnedMatchesUpstream) is gated behind the
// `specnetwork` build tag and is the only network-touching member of
// the package.
package specserver

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// Spec is the parsed swagger document the test server validates
// against. Hold by value; methods are read-only.
type Spec struct {
	// Path is the absolute filesystem path the spec was loaded from.
	// Surfaced in diagnostic error messages so a developer can grep
	// "which spec snapshot did the failure come from".
	Path string

	// Operations maps "METHOD /path" → Operation. Path is the spec's
	// path-template literal; the test server matches request paths
	// against templated keys (the SDK currently has no
	// path-parameter endpoints, so literal match suffices).
	Operations map[string]Operation

	// raw is retained for $ref resolution in the walker.
	raw *rawSpec
}

// Operation captures the shape an inbound request must have to pass
// validation. The walker computes RequiredBody / RequiredQuery once
// at Load time; the dispatcher consults them per request.
type Operation struct {
	// Method is uppercase HTTP verb ("POST", "GET").
	Method string

	// Path is the path-template literal as it appears in the spec
	// (e.g. "/auth", "/<authNotifyUrl>"). The dispatcher matches
	// this verbatim — partner-hosted callback paths are out of
	// scope for the spec server.
	Path string

	// RequiredBody is the flattened set of required-field paths in
	// the request body. Top-level fields appear as "fieldName";
	// nested via $ref-array members appear as "outer[].inner".
	// Dotted-property nesting appears as "outer.inner".
	RequiredBody []string

	// RequiredQuery is the set of GET query param names the spec
	// marks as required.
	RequiredQuery []string

	// RequiredHeader is the set of header names the spec declares
	// `in: header, required: true`. Today's spec uses this for the
	// Encrypt header on the two credit POSTs (Q31–Q34 hybrid
	// encryption). When the dispatcher sees an Encrypt header on a
	// POST, body validation is bypassed because the body is
	// AES-ECB ciphertext that the spec server cannot decrypt.
	//
	// The Authorization header is filtered out by the walker —
	// every signed endpoint declares it required, but signature
	// validation is out of scope per architect §1.7.
	RequiredHeader []string
}

// rawSpec is the small slice of swagger.yaml the walker actually
// needs — paths and component definitions. We deliberately do NOT
// model the full OpenAPI 3.0 schema; the walker dives into the
// generic yaml.Node tree instead, which is robust against the
// extensions and dialect quirks Atome's spec ships with.
type rawSpec struct {
	Paths      map[string]yaml.Node `yaml:"paths"`
	Components struct {
		RequestBodies map[string]yaml.Node `yaml:"requestBodies"`
		Schemas       map[string]yaml.Node `yaml:"schemas"`
	} `yaml:"components"`
}

// Load parses a pinned swagger.yaml at the given path and returns a
// Spec ready for server.go to query. Returns an error if the file
// is unreadable, malformed YAML, or doesn't look like an OpenAPI 3
// document.
func Load(path string) (*Spec, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("specserver: resolve %q: %w", path, err)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("specserver: read %q: %w", abs, err)
	}
	var raw rawSpec
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("specserver: parse %q: %w", abs, err)
	}
	if len(raw.Paths) == 0 {
		return nil, fmt.Errorf("specserver: %q: no paths section (not an OpenAPI document?)", abs)
	}
	s := &Spec{
		Path:       abs,
		Operations: make(map[string]Operation, len(raw.Paths)*2),
		raw:        &raw,
	}
	if err := s.walk(); err != nil {
		return nil, fmt.Errorf("specserver: walk %q: %w", abs, err)
	}
	return s, nil
}

// LoadDefault loads the single pinned swagger.yaml from
// qa/specserver/testdata/. The package ships exactly one pinned
// snapshot at a time; bumping it is a deliberate commit (rename the
// file with the new SHA prefix and Load picks it up automatically).
func LoadDefault() (*Spec, error) {
	dir, err := defaultTestdataDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("specserver: read testdata dir %q: %w", dir, err)
	}
	var pinned string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasPrefix(name, "swagger-") &&
			(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			if pinned != "" {
				return nil, fmt.Errorf("specserver: multiple pinned swagger files in %q (have %q and %q); ship exactly one", dir, pinned, name)
			}
			pinned = name
		}
	}
	if pinned == "" {
		return nil, fmt.Errorf("specserver: no pinned swagger-*.yaml in %q", dir)
	}
	return Load(filepath.Join(dir, pinned))
}

// PinnedPath returns the absolute path of the single pinned swagger
// file under qa/specserver/testdata/. Used by drift_test.go to
// SHA-compare against the upstream fetch.
func PinnedPath() (string, error) {
	dir, err := defaultTestdataDir()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("specserver: read testdata dir %q: %w", dir, err)
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasPrefix(name, "swagger-") &&
			(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			return filepath.Join(dir, name), nil
		}
	}
	return "", fmt.Errorf("specserver: no pinned swagger-*.yaml in %q", dir)
}

// defaultTestdataDir resolves the qa/specserver/testdata/ directory
// relative to this source file. Robust against `go test` running
// from any cwd in the module.
func defaultTestdataDir() (string, error) {
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("specserver: runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(here), "testdata"), nil
}

// Op returns the Operation registered for "METHOD /path". Method is
// case-insensitive on input but stored uppercase. Returns the zero
// Operation and ok=false if the spec doesn't declare this op.
func (s *Spec) Op(method, path string) (Operation, bool) {
	if s == nil {
		return Operation{}, false
	}
	op, ok := s.Operations[opKey(method, path)]
	return op, ok
}

// Endpoints returns every "METHOD /path" the spec declares. Order is
// stable — useful for "endpoint in spec but not covered" surfacing.
func (s *Spec) Endpoints() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.Operations))
	for k := range s.Operations {
		out = append(out, k)
	}
	// Stable sort so test output is reproducible.
	stableSort(out)
	return out
}

func opKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

// stableSort is a tiny insertion sort — avoids pulling in `sort` for
// the small lists this package handles (≤ ~40 endpoints).
func stableSort(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
