package mock

import (
	"container/list"
	"net/http"
	"sync"
	"time"
)

// ServerOption configures a v0.5 mock.Server beyond the basic
// Scenario dispatch shipped in v0.4. All shipped options are
// **off by default** so every v0.4 test continues to work
// unchanged when only `mock.NewServer(t, scenario)` is called.
//
// The four sandbox-realism extensions per V0.5_DESIGN.md §2:
//
//   - WithSpecValidation       — pre-validate against pinned spec
//   - WithIdempotency          — replay cache keyed on requestId
//   - WithAutoCallback         — fire matching *Event after sync
//   - WithResponseSigning      — sign response bodies (forward-compat)
type ServerOption func(*serverConfig)

type serverConfig struct {
	specValidation        bool
	idempotency           bool
	idempotencyCacheSize  int
	autoCallbackHandlers  map[string]http.Handler
	autoCallbackURL       string
	autoCallbackDelay     time.Duration
	autoCallbackSignerPEM []byte
	responseSigningKeyPEM []byte
}

// WithSpecValidation pre-validates every inbound request against
// the pinned upstream swagger.yaml: a missing required header,
// query param, or body field returns 400 PARAMS_MISSING the same
// way the upstream gateway does. Off by default — partners
// asserting on retries / signatures don't want their tests
// rejected by spec drift before reaching their assertion.
//
// Spec validation is presence-only (matches qa/specserver
// behaviour); type / enum / maxLength checks remain out of scope.
// The pinned spec lives at `internal/spec/testdata/`; bumping it
// is a deliberate human action.
func WithSpecValidation() ServerOption {
	return func(c *serverConfig) {
		c.specValidation = true
	}
}

// WithIdempotency enables the replay cache keyed on
// (method, path, requestId). A duplicate within the cache window
// returns the original response byte-for-byte — matches the
// expected Atome gateway semantic (option (a) in
// V0.5_DESIGN.md §9-2; partner-pending Q).
//
// Off by default. A test asserting that the SDK retries 3× on
// 5xx wants each attempt to count separately; flipping the cache
// on for that test would mask the bug.
//
// `requestId` extraction:
//   - POST: parse body, look up top-level `requestId`
//   - GET:  read from query parameter
//   - Encrypted POST: NOT yet supported — the Server has no
//     decryption key, so encrypted requests bypass the cache.
//     (Acceptable for v0.5: encrypted endpoints are credit-only;
//     v0.6 can wire decrypt-then-cache once partners need it.)
//
// Cache: bounded LRU, default 1024 entries, no TTL. Override
// the size with WithIdempotencyCacheSize. Server.Reset() clears it.
func WithIdempotency() ServerOption {
	return func(c *serverConfig) {
		c.idempotency = true
		if c.idempotencyCacheSize == 0 {
			c.idempotencyCacheSize = 1024
		}
	}
}

// WithIdempotencyCacheSize configures the LRU cache cap when
// idempotency is enabled. Default 1024. Has no effect if
// WithIdempotency was not also passed.
func WithIdempotencyCacheSize(n int) ServerOption {
	return func(c *serverConfig) {
		if n > 0 {
			c.idempotencyCacheSize = n
		}
	}
}

// WithAutoCallback fires the matching `*Event` to the partner's
// callback handler after the synchronous response is written.
// `handlerByOp` is keyed on the SAME "METHOD path" shape as
// PerEndpoint; only ops with a registered handler trigger a
// callback fire.
//
// In-process default: handler is invoked via ServeHTTP on a
// httptest.NewRecorder, sharing the v0.4 Fire*Callback signing
// machinery. No network race; deterministic ordering.
//
// Off by default — most v0.4 tests don't expect callback traffic.
//
// The auto-callback firing reuses the bundled mock signing key
// when `WithAutoCallbackKey` is not set; without bundled-key
// opt-in the firing logs a setup error via Server.Failures and
// no callback is sent.
func WithAutoCallback(handlerByOp map[string]http.Handler) ServerOption {
	return func(c *serverConfig) {
		out := make(map[string]http.Handler, len(handlerByOp))
		for k, h := range handlerByOp {
			out[normalizeOpKey(k)] = h
		}
		c.autoCallbackHandlers = out
	}
}

// WithCallbackURL routes auto-callbacks via a real HTTP POST to
// `url` instead of in-process ServeHTTP. Useful when the
// partner's callback handler runs in a separate dev process or
// behind a proxy. Mutually exclusive with WithAutoCallback (if
// both are passed, the in-process map wins).
func WithCallbackURL(url string) ServerOption {
	return func(c *serverConfig) {
		c.autoCallbackURL = url
	}
}

// WithCallbackDelay introduces a synthetic delay between the
// sync response and the auto-callback fire. Useful for tests
// asserting partner-side race-condition behaviour (e.g. callback
// arrives mid-poll). Default 0 (callback fires synchronously
// before the sync response returns to the SDK).
//
// Open question for partner (V0.5_DESIGN.md §9-3): what's a
// realistic upstream callback delay? Default updated when the
// partner closes Q.
func WithCallbackDelay(d time.Duration) ServerOption {
	return func(c *serverConfig) {
		c.autoCallbackDelay = d
	}
}

// WithAutoCallbackKey sets the RSA private key the auto-callback
// firing uses to sign the outbound callback body. PEM bytes,
// PKCS#1 or PKCS#8. When unset, auto-callback firing falls back
// to the bundled mock signing key — which only works if the
// partner-side verifier was built with `MockSigningPubCertPEM`.
func WithAutoCallbackKey(privPEM []byte) ServerOption {
	return func(c *serverConfig) {
		c.autoCallbackSignerPEM = privPEM
	}
}

// WithResponseSigning signs every response body with the
// supplied RSA private key (PEM) and emits the signature in the
// `Authorization` response header.
//
// **Forward-compat plumbing only.** The SDK does not verify
// outbound-response signatures today (Q5 partner-pending). Off
// by default; flipping it on lets partners running
// "what does v0.6 look like?" tests exercise the
// response-verification side without changing production code.
func WithResponseSigning(privPEM []byte) ServerOption {
	return func(c *serverConfig) {
		c.responseSigningKeyPEM = privPEM
	}
}

// normalizeOpKey turns a public-API "Method /path" string into
// the internal op-key shape (uppercase method, verbatim path).
// Mirrors the same helper in qa/specserver — the mock package
// keeps its own copy so it has zero qa/ imports.
func normalizeOpKey(op string) string {
	for i := 0; i < len(op); i++ {
		if op[i] == ' ' {
			return upperASCII(op[:i]) + op[i:]
		}
	}
	return op
}

// upperASCII returns s with ASCII a-z mapped to A-Z. Used by
// normalizeOpKey only — HTTP methods are ASCII.
func upperASCII(s string) string {
	out := []byte(s)
	for i := range out {
		if out[i] >= 'a' && out[i] <= 'z' {
			out[i] -= 'a' - 'A'
		}
	}
	return string(out)
}

// idempotencyCache is a tiny LRU keyed on string (the
// `(method, path, requestId)` composite). Stores byte-frozen
// response state so retries get the same bytes.
type idempotencyCache struct {
	mu    sync.Mutex
	cap   int
	items map[string]*list.Element
	order *list.List
}

type cacheEntry struct {
	key     string
	status  int
	body    []byte
	headers http.Header
}

func newIdempotencyCache(capacity int) *idempotencyCache {
	if capacity <= 0 {
		capacity = 1024
	}
	return &idempotencyCache{
		cap:   capacity,
		items: make(map[string]*list.Element, capacity),
		order: list.New(),
	}
}

// Get returns the cached entry for key (refreshing LRU order)
// or nil if absent.
func (c *idempotencyCache) Get(key string) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		return elem.Value.(*cacheEntry)
	}
	return nil
}

// Put inserts (or overwrites) the cached entry; evicts the LRU
// item if at capacity.
func (c *idempotencyCache) Put(key string, e *cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		elem.Value = e
		c.order.MoveToFront(elem)
		return
	}
	if c.order.Len() >= c.cap {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*cacheEntry).key)
		}
	}
	elem := c.order.PushFront(e)
	c.items[key] = elem
}

// Reset clears the cache.
func (c *idempotencyCache) Reset() {
	c.mu.Lock()
	c.items = make(map[string]*list.Element, c.cap)
	c.order = list.New()
	c.mu.Unlock()
}
