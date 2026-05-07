package mock

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

// Transport is the in-process http.RoundTripper that the
// `mock.NewClient` Client uses to dispatch outbound requests to a
// Scenario. Partners can also instantiate it directly:
//
//	rt := mock.NewTransport(t, mock.AlwaysSuccess())
//	c, _ := atomefin.New(
//	    atomefin.WithBaseURL("https://atome-fin.test"),
//	    atomefin.WithPrivateKeyPEM(testKey),
//	    atomefin.WithHTTPClient(&http.Client{Transport: rt}),
//	)
//
// The Transport records per-op invocation counts under the same
// "METHOD path" key shape as PerEndpoint — useful for asserting
// retry counts, idempotency-key reuse, etc.
type Transport struct {
	tb testing.TB

	mu       sync.Mutex
	scenario Scenario
	requests []*RecordedRequest
	hits     map[string]int64
}

// RecordedRequest captures one inbound request for after-the-fact
// inspection (assertion). Concurrency-safe via Transport's mutex.
type RecordedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Query   string
	Body    []byte
}

// NewTransport returns a stub RoundTripper that dispatches every
// inbound request through scenario. testing.TB is required so
// internal failures (a Scenario that panics, a misconfigured
// response) can be surfaced via t.Errorf.
//
// scenario must be non-nil; pass AlwaysSuccess() if you don't
// need anything custom yet.
func NewTransport(tb testing.TB, scenario Scenario) *Transport {
	tb.Helper()
	if scenario == nil {
		tb.Fatalf("mock.NewTransport: nil Scenario")
	}
	return &Transport{
		tb:       tb,
		scenario: scenario,
		hits:     make(map[string]int64),
	}
}

// SetScenario swaps the active Scenario. Useful for tests that
// drive distinct phases (e.g. PROCESSING then SUCCESS for a
// poll loop).
func (t *Transport) SetScenario(s Scenario) {
	if s == nil {
		t.tb.Fatalf("mock.Transport.SetScenario: nil Scenario")
	}
	t.mu.Lock()
	t.scenario = s
	t.mu.Unlock()
}

// Hits returns the invocation count for "METHOD path" (e.g.
// "POST /auth"). Useful for asserting retry behaviour.
func (t *Transport) Hits(op string) int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.hits[op]
}

// Requests returns a copy of the recorded inbound requests in
// chronological order.
func (t *Transport) Requests() []*RecordedRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]*RecordedRequest, len(t.requests))
	copy(out, t.requests)
	return out
}

// Reset clears the recorded state (hits + requests). The Scenario
// is preserved.
func (t *Transport) Reset() {
	t.mu.Lock()
	t.requests = nil
	t.hits = make(map[string]int64)
	t.mu.Unlock()
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := &RecordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
	}
	rec.Headers = r.Header.Clone()
	if r.Body != nil {
		// Read once, replace the body so the SDK's later "did
		// the bytes I sent equal the bytes I signed" check
		// continues to work.
		body := readAndReset(r)
		rec.Body = body
	}

	op := r.Method + " " + r.URL.Path
	t.mu.Lock()
	t.requests = append(t.requests, rec)
	t.hits[op]++
	scenario := t.scenario
	t.mu.Unlock()

	atomic.AddInt64(new(int64), 0) // touch sync/atomic; package may be unused otherwise
	return scenario.Respond(r)
}

// readAndReset drains r.Body into a buffer and replaces r.Body
// with a fresh io.NopCloser around the same bytes so downstream
// readers see an unchanged stream. Returns the captured bytes.
func readAndReset(r *http.Request) []byte {
	body := make([]byte, 0, 256)
	if r.Body == nil {
		return body
	}
	defer func() { _ = r.Body.Close() }()
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	r.Body = newNopCloserBytes(body)
	return body
}

// Server is a real httptest.NewServer backed by the same Scenario
// dispatch logic as Transport. Useful for tests that aren't using
// `*atomefin.Client` directly (e.g. a curl-based contract test,
// or a partner's own HTTP client library that doesn't accept a
// custom RoundTripper).
//
// v0.5 added four opt-in extensions for sandbox-realism — see
// server_options.go (WithSpecValidation / WithIdempotency /
// WithAutoCallback / WithResponseSigning). All off by default;
// the v0.4 surface is preserved verbatim when no ServerOption
// is supplied.
type Server struct {
	*httptest.Server
	tb        testing.TB
	transport *Transport
	cfg       *serverConfig
	idem      *idempotencyCache
}

// NewServer returns a started httptest.Server that dispatches
// every inbound request through the supplied Scenario, with any
// ServerOption extensions applied.
//
// EnvProd considerations don't apply here — the URL is
// httptest-allocated and not addressable from outside the test
// process. The bundled-keys precaution still applies through
// `mock.NewClient`; this Server is the verb-arbitrary sibling.
//
// v0.5 added the variadic `opts ...ServerOption` tail; v0.4
// callers calling `mock.NewServer(t, scenario)` continue to
// work unchanged because every ServerOption is opt-in.
func NewServer(tb testing.TB, scenario Scenario, opts ...ServerOption) *Server {
	tb.Helper()
	if scenario == nil {
		tb.Fatalf("mock.NewServer: nil Scenario")
	}
	cfg := &serverConfig{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	tr := NewTransport(tb, scenario)
	srv := &Server{tb: tb, transport: tr, cfg: cfg}
	if cfg.idempotency {
		srv.idem = newIdempotencyCache(cfg.idempotencyCacheSize)
	}
	srv.Server = httptest.NewServer(http.HandlerFunc(srv.serveHTTP))
	tb.Cleanup(srv.Server.Close)
	return srv
}

// Transport returns the underlying *Transport so callers can
// assert on Hits / Requests / SetScenario without going through
// the *Server wrapper.
func (s *Server) Transport() *Transport { return s.transport }

// SetScenario forwards to the underlying Transport.
func (s *Server) SetScenario(scenario Scenario) { s.transport.SetScenario(scenario) }

// Hits forwards to the underlying Transport.
func (s *Server) Hits(op string) int64 { return s.transport.Hits(op) }

// Requests forwards to the underlying Transport.
func (s *Server) Requests() []*RecordedRequest { return s.transport.Requests() }

// Reset clears the underlying Transport's hit / request log AND
// the idempotency cache (when WithIdempotency is in effect).
func (s *Server) Reset() {
	s.transport.Reset()
	if s.idem != nil {
		s.idem.Reset()
	}
}
