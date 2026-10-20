#!/usr/bin/env bash
# Fuzz the external API against its OpenAPI contract with Schemathesis (schemathesis.toml): boot
# the server on the seeded database of this checkout, run Schemathesis as the anchor API token of
# the fixtures, stop the server. The rate limits are off (the failed-authentication one too: the
# fuzzer sends bad credentials on purpose) so a 429 does not stand in for an answer.
set -euo pipefail

port="${CONTRACT_PORT:-3990}"
dir="$(mktemp -d)"
log="$dir/server.log"
# fixtures/api_tokens.yml ContractToken: every scope, so no operation answers 403. The omtk_ token
# format is omtk_<prefix>_<secret>.
token="omtk_contractfz01_contract-token-secret"

# The compiled binary runs directly: `go run` would leave the server it starts behind when stopped.
go build -o "$dir/server" ./cmd/server
PORT="$port" SPHERICON_RATE_LIMIT_API_BURST_PER_SECOND=0 SPHERICON_RATE_LIMIT_API_PER_MINUTE=0 \
  SPHERICON_RATE_LIMIT_FAILED_AUTH_PER_MINUTE=0 \
  "$dir/server" >"$log" 2>&1 &
server=$!
trap 'kill "$server" 2>/dev/null || true; rm -rf "$dir"' EXIT

until curl -fsS "http://localhost:$port/healthz" >/dev/null 2>&1; do
  kill -0 "$server" 2>/dev/null || { cat "$log"; exit 1; }
  sleep 1
done

status=0
uvx schemathesis run openapi/external.openapi.json \
  --url "http://localhost:$port/api" \
  -H "Authorization: Bearer $token" \
  --max-examples 25 --workers 2 || status=$?

# A 500 tells the client nothing; what the server logged says what failed.
grep '"msg":"request failed"' "$log" | sort -u | head -20 || true
exit "$status"
