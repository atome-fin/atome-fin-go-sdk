#!/usr/bin/env bash
# Run credit_information_pre with pre-prod keys.
# Usage:
#   ./scripts/run_credit_information_pre.sh          # live pre request
#   ./scripts/run_credit_information_pre.sh dry-run  # local crypto check only

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
KEY_DIR="$ROOT/examples/credit_information_pre/.local-keys"
BIN="$ROOT/credit_information_pre"

export ATOME_FIN_PRIV_KEY_PEM="$KEY_DIR/grab_sign_priv.pem"
export ATOME_FIN_ATOME_ENCRYPT_CERT_PEM="$KEY_DIR/atome_encrypt_pub.pem"

if [[ "${1:-}" == "dry-run" ]]; then
  export ATOME_FIN_DRY_RUN=1
fi

if [[ ! -x "$BIN" ]]; then
  echo "binary not found: $BIN"
  echo "build it first:"
  echo "  go build -o credit_information_pre ./examples/credit_information_pre/"
  exit 1
fi

exec "$BIN"
