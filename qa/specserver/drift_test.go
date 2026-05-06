//go:build specnetwork

package specserver_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// upstreamSwaggerURL is the canonical source of the partner spec.
// Documented in docs/internal/SPEC_DELTA_2026-05-06.md §0.
const upstreamSwaggerURL = "https://doc.apaylater.net/white-label/G/swagger.yaml"

// allowlistFile carries SHA-256 prefixes (one per line, optionally
// followed by `# rationale`) for upstream snapshots that drift
// detection should NOT fail on. Used during in-flight spec bumps
// when the pinned SHA hasn't caught up yet, or when the upstream
// has temporarily reverted to a known-prior shape during a partner-
// side hotfix window. Empty file ⇒ strict pinned-only check.
const allowlistFile = "spec_drift_allowlist.txt"

// TestSpec_PinnedMatchesUpstream is the spec-drift sentinel.
//
// Build-tagged behind `specnetwork` so it does NOT run on a default
// `go test ./...` — keeps dev and unit-CI hermetic. Enable in CI
// (or locally) with:
//
//	go test -tags specnetwork ./qa/specserver/
//
// or via the Makefile's `make test-spec-drift` target.
//
// On run: fetches the upstream swagger.yaml, SHA-256s the bytes,
// and compares against the SHA prefix in the pinned file's
// filename. A mismatch is a HARD FAIL by default — CI surfaces it
// with the upstream SHA + the pinned SHA + a pointer to the bump
// workflow in SPEC_ASSERTION_TEST_DESIGN.md §1.5. The allowlist
// (qa/specserver/spec_drift_allowlist.txt) carries SHA prefixes to
// suppress for known in-flight transitions.
//
// Network failures (DNS, timeout, non-2xx) are treated as
// inconclusive and SKIP the test rather than fail — the framework
// is not a network-availability monitor and a transient gateway
// outage shouldn't redden CI.
func TestSpec_PinnedMatchesUpstream(t *testing.T) {
	pinnedPath, err := specserver.PinnedPath()
	if err != nil {
		t.Fatalf("locate pinned spec: %v", err)
	}
	pinnedSHA, err := pinnedSHAFromFilename(pinnedPath)
	if err != nil {
		t.Fatalf("parse pinned filename: %v", err)
	}
	t.Logf("pinned spec: %s (sha256-prefix %s)", filepath.Base(pinnedPath), pinnedSHA)

	// Fetch upstream — short timeout, single attempt. Treat any
	// transport / non-2xx as inconclusive.
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, upstreamSwaggerURL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("upstream unreachable (%v); skipping drift check — re-run when connectivity is restored", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("upstream returned HTTP %d; skipping drift check — re-run when upstream is healthy", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Skipf("read upstream body: %v", err)
	}

	digest := sha256.Sum256(body)
	upstreamSHA := hex.EncodeToString(digest[:])

	t.Logf("upstream sha256: %s (%d bytes)", upstreamSHA, len(body))

	// Allowlist check first.
	allowlisted, why, err := isAllowlisted(filepath.Dir(pinnedPath), upstreamSHA)
	if err != nil {
		t.Fatalf("read allowlist: %v", err)
	}
	if allowlisted {
		t.Logf("upstream SHA matches allowlist entry: %s", why)
		return
	}

	// Strict-prefix compare — pinned filename carries an 8-hex
	// prefix; upstream must start with the same prefix (i.e. the
	// pinned spec IS the upstream). A bump = rename the pinned
	// file; this test then re-passes.
	if !strings.HasPrefix(upstreamSHA, pinnedSHA) {
		t.Errorf(`upstream swagger drifted from pinned snapshot

  pinned sha256-prefix: %s   (file: %s)
  upstream sha256:      %s

What to do:
  1. Architect: re-pull and write SPEC_DELTA_<date>.md describing
     the change.
  2. Lead-coder: bump qa/specserver/testdata/swagger-*.yaml — copy
     the new bytes in and rename the file with the new sha-prefix.
  3. Re-run go test ./... — every endpoint that the spec change
     affects will surface as a localized *_spec_test.go failure.
  4. If shipping in-flight bumps, add the upstream sha-prefix to
     %s with a one-line rationale comment.

See docs/internal/SPEC_ASSERTION_TEST_DESIGN.md §1.5 for the full
workflow.`, pinnedSHA, filepath.Base(pinnedPath), upstreamSHA, allowlistFile)
	}
}

// pinnedSHAFromFilename extracts the SHA-prefix from a filename
// shaped `swagger-YYYY-MM-DD-<hex>.yaml`. Returns the lowercase hex
// substring.
func pinnedSHAFromFilename(path string) (string, error) {
	base := filepath.Base(path)
	// Strip extension.
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	parts := strings.Split(base, "-")
	if len(parts) < 5 || parts[0] != "swagger" {
		return "", errors.New("filename must be swagger-YYYY-MM-DD-<sha>.yaml")
	}
	return strings.ToLower(parts[len(parts)-1]), nil
}

// isAllowlisted reads the optional allowlist file and reports
// whether `upstreamSHA` is present (prefix-match against any
// allowlist line). Returns the matched line as `why` for the
// caller's log. Empty / missing allowlist ⇒ no-op.
func isAllowlisted(dir, upstreamSHA string) (bool, string, error) {
	path := filepath.Join(dir, "..", allowlistFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		// Split on first `#` for inline comment / rationale.
		sha := raw
		if i := strings.IndexByte(raw, '#'); i >= 0 {
			sha = strings.TrimSpace(raw[:i])
		}
		sha = strings.ToLower(sha)
		if sha != "" && strings.HasPrefix(strings.ToLower(upstreamSHA), sha) {
			return true, raw, nil
		}
	}
	return false, "", nil
}
