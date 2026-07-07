package atomefin

import "fmt"

// Environment names a deployment of the atome-fin Partner API.
//
// Partners can pin a confirmed URL with WithBaseURL() once they receive
// it from Atome ops without recompiling.
type Environment string

// Environments for pre-production (联调) and production. EnvProd's host
// (`api.atome.id`) differs from the pre host (`apaylater.net`).
const (
	EnvPre  Environment = "pre"
	EnvProd Environment = "prod"
)

// baseURLs maps each Environment to its base URL. The map is internal so
// callers cannot accidentally mutate it across goroutines.
var baseURLs = map[Environment]string{
	EnvPre:  "https://id-api-pre.apaylater.net/grabpaylater",
	EnvProd: "https://api.atome.id/grabpaylater",
}

// BaseURL returns the base URL bound to env. Returns ("", error) if env is
// not one of EnvPre / EnvProd.
//
// Use WithBaseURL() to override at Client construction time. The two
// options compose: WithEnvironment selects a default; WithBaseURL — if
// also passed — wins and overrides whatever was set.
func BaseURL(env Environment) (string, error) {
	u, ok := baseURLs[env]
	if !ok {
		return "", fmt.Errorf("atomefin: unknown environment %q (want one of pre/prod)", env)
	}
	return u, nil
}
