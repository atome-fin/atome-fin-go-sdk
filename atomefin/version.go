package atomefin

// SDKVersion is the semantic version reported by the SDK in its default
// User-Agent header and observability hooks.
//
// Pre-1.0 (v0.x): minor versions may break — the upstream API is itself
// tagged "Draft" (see DESIGN.md §12). Tag v0.1.0 is reserved for the first
// complete auth/capture/voidAuth cut.
const SDKVersion = "0.1.0"

// Version returns SDKVersion. Exposed as a function so callers that want to
// embed it into structured logs can take its address (`var v = atomefin.Version`).
func Version() string { return SDKVersion }
