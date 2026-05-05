# atome-fin-go-sdk — top-level developer Makefile.
#
# All targets are stdlib-Go-friendly (no third-party Make tooling). Run
# `make help` for the canonical list.

GO       ?= go
PKG      ?= ./...
GOLINT   ?= golangci-lint
GOVULN   ?= govulncheck

# Pinned versions — match .github/workflows/ci.yml so a clean local
# `make ci` produces the same verdict as a fresh CI run.
GOLINT_VERSION ?= v1.64.8
GOVULN_VERSION ?= latest

.PHONY: help build test test-race lint vet fmt fmtcheck cover examples \
        sandbox-smoke sandbox-webhook clean ci govulncheck

# Self-documenting help: each target carries its docstring INLINE after
# the colon as `target: ## doc string`. The awk filter below expects
# that exact shape — keep new targets in the same form.
help: ## show this list
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_\-]+:.*## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## compile every package + example
	$(GO) build $(PKG)
	$(GO) build ./examples/...

test: ## run the full test suite
	$(GO) test $(PKG)

test-race: ## run the suite with the race detector
	$(GO) test -race $(PKG)

lint: ## golangci-lint run (REQUIRED — install hint printed if missing)
	@if ! command -v $(GOLINT) >/dev/null 2>&1; then \
		echo "ERROR: golangci-lint not installed (CI uses $(GOLINT_VERSION))."; \
		echo ""; \
		echo "Install (matching the pinned CI version):"; \
		echo "  go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLINT_VERSION)"; \
		echo ""; \
		echo "Then ensure \$$GOPATH/bin (or \$$GOBIN) is on your PATH and re-run:"; \
		echo "  make lint"; \
		exit 1; \
	fi
	$(GOLINT) run $(PKG)

govulncheck: ## run govulncheck (REQUIRED — install hint printed if missing)
	@if ! command -v $(GOVULN) >/dev/null 2>&1; then \
		echo "ERROR: govulncheck not installed (CI uses $(GOVULN_VERSION))."; \
		echo ""; \
		echo "Install:"; \
		echo "  go install golang.org/x/vuln/cmd/govulncheck@$(GOVULN_VERSION)"; \
		echo ""; \
		echo "Then ensure \$$GOPATH/bin (or \$$GOBIN) is on your PATH and re-run:"; \
		echo "  make govulncheck"; \
		exit 1; \
	fi
	$(GOVULN) ./...

vet: ## go vet
	$(GO) vet $(PKG)

fmt: ## rewrite source with gofmt
	gofmt -w .

fmtcheck: ## fail if any tracked file is not gofmt-clean
	@diff=$$(gofmt -l .); if [ -n "$$diff" ]; then \
		echo "gofmt diff in:"; echo "$$diff"; \
		echo "run \`make fmt\`"; exit 1; \
	fi

cover: ## run tests with coverage and print per-package totals
	$(GO) test -cover $(PKG)

examples: ## build the two example commands
	$(GO) build ./examples/auth_capture/
	$(GO) build ./examples/webhook_server/

sandbox-smoke: ## run examples/auth_capture against the test env (gated by ATOME_FIN_RUN_SMOKE=1)
	@if [ "$$ATOME_FIN_RUN_SMOKE" != "1" ]; then \
		echo "set ATOME_FIN_RUN_SMOKE=1 to actually hit the test environment"; \
		echo "required env vars:"; \
		echo "  ATOME_FIN_PRIV_KEY_PEM  — partner RSA-2048 private key (PEM path)"; \
		echo "  ATOME_FIN_SESSION_ID    — sessionid for /auth (max 64)"; \
		echo "  ATOME_FIN_EXTERNAL_UID  — partner-side user id"; \
		echo "  (optional) ATOME_FIN_PARTNER_ID ATOME_FIN_BASE_URL ATOME_FIN_RUN_CAPTURE"; \
	else \
		$(GO) run ./examples/auth_capture/; \
	fi

sandbox-webhook: ## boot examples/webhook_server (long-running)
	@if [ -z "$$ATOME_FIN_ATOME_CERT_PEM" ] && [ -z "$$ATOME_FIN_ATOME_CERT_PEMS" ]; then \
		echo "set ATOME_FIN_ATOME_CERT_PEM (single) or ATOME_FIN_ATOME_CERT_PEMS (multi-cert)"; \
		exit 1; \
	fi
	$(GO) run ./examples/webhook_server/

clean: ## remove built example binaries
	rm -f auth_capture webhook_server

ci: ## run the same gates GitHub Actions runs — fmtcheck, vet, build, test-race, lint, govulncheck
	@echo "==> fmtcheck"  && $(MAKE) --no-print-directory fmtcheck
	@echo "==> vet"        && $(MAKE) --no-print-directory vet
	@echo "==> build"      && $(MAKE) --no-print-directory build
	@echo "==> test-race"  && $(MAKE) --no-print-directory test-race
	@echo "==> lint"       && $(MAKE) --no-print-directory lint
	@echo "==> govulncheck" && $(MAKE) --no-print-directory govulncheck
	@echo ""
	@echo "ci: all green — push with confidence"
