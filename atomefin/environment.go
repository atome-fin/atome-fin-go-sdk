package atomefin

import "fmt"

// Environment names a deployment of the atomefin white-label "G" API.
//
// All three URLs in the spec are explicitly tagged as **placeholders** to be
// reconfirmed before go-live (DESIGN.md §13/Q1). The constants below ship
// the spec values verbatim — partners can pin a confirmed URL with
// WithBaseURL() once they receive it from Atome ops without recompiling.
type Environment string

// Environments declared in the spec. EnvProd's host (`api.atome.id`) differs
// from the doc host (`apaylater.net`) — Q1 covers whether routing is
// per-country.
const (
	EnvTest Environment = "test"
	EnvPre  Environment = "pre"
	EnvProd Environment = "prod"
)

// baseURLs maps each Environment to its placeholder base URL. The map is
// internal so callers cannot accidentally mutate it across goroutines.
var baseURLs = map[Environment]string{
	EnvTest: "https://id-api.apaylater.net/white-label/G",
	EnvPre:  "https://id-api-pre.apaylater.net/white-label/G",
	EnvProd: "https://api.atome.id/white-label/G",
}

// BaseURL returns the placeholder base URL bound to env. Returns
// ("", error) if env is not one of EnvTest / EnvPre / EnvProd.
//
// Use WithBaseURL() to override at Client construction time. The two
// options compose: WithEnvironment selects a default; WithBaseURL — if
// also passed — wins and overrides whatever was set.
func BaseURL(env Environment) (string, error) {
	u, ok := baseURLs[env]
	if !ok {
		return "", fmt.Errorf("atomefin: unknown environment %q (want one of test/pre/prod)", env)
	}
	return u, nil
}
