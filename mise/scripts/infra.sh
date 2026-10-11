#!/usr/bin/env bash
# Run Terraform in infra/ with credentials from the `aws login` session
# (mise run infra -- init|plan|apply|destroy|output ...). Nothing is read from files and nothing
# secret is printed: the state bucket name is the only -backend-config and is not a secret.
set -euo pipefail
set +x
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=infra-common.sh
source "$here/infra-common.sh"

require_tools terraform
aws_login

infra_dir="$here/../../infra"
tf=(terraform -chdir="$infra_dir")
backend=(-backend-config="bucket=$(state_bucket)")
export TF_INPUT=0

cmd="${1:-}"
[[ -n "$cmd" ]] || die "usage: mise run infra -- init|plan|apply|destroy|output ..."
shift

if [[ "$cmd" == init ]]; then
  exec "${tf[@]}" init "${backend[@]}" "$@"
fi
# Initialise by itself when the s3 backend is not configured yet.
if ! grep -qs '"backend"' "$infra_dir/.terraform/terraform.tfstate"; then
  "${tf[@]}" init "${backend[@]}" >&2
fi

# Non-secret values come from infra/production.tfvars (gitignored). `apply <planfile>` takes no
# -var-file: a first argument that is not a flag is a saved plan.
vars=()
case "$cmd" in
  plan | destroy | import | refresh) vars=(-var-file=production.tfvars) ;;
  apply) [[ $# -eq 0 || "${1:-}" == -* ]] && vars=(-var-file=production.tfvars) ;;
  *) ;;
esac
exec "${tf[@]}" "$cmd" ${vars[@]+"${vars[@]}"} "$@"
