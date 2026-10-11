#!/usr/bin/env bash
# Store the Enterprise Edition LICENSE_KEY in the app secret (optional; empty runs the core).
# The key is read from stdin (never argv or shell history): `mise run infra:license < key.txt`,
# or paste it and press Ctrl-D. The other keys of the secret are kept as they are.
set -euo pipefail
set +x
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=infra-common.sh
source "$here/infra-common.sh"

require_tools jq
aws_login

license="$(tr -d '[:space:]')"
[[ -n "$license" ]] || die "no license key on stdin"
current="$(aws secretsmanager get-secret-value --secret-id "$INFRA_APP_SECRET" \
  --query SecretString --output text)" || die "secret '$INFRA_APP_SECRET' not found: run mise run infra:bootstrap"
jq -c --arg k "$license" '.LICENSE_KEY = $k' <<<"$current" |
  aws secretsmanager put-secret-value --secret-id "$INFRA_APP_SECRET" \
    --secret-string file:///dev/stdin >/dev/null
echo "LICENSE_KEY stored; tasks started from now on read it (restart the service to apply it now)"
