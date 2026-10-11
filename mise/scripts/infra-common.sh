#!/usr/bin/env bash
# Shared by the infra scripts (sourced, not run). Nothing secret is ever printed or put in argv.
# shellcheck disable=SC2034  # the sourcing scripts read these variables

INFRA_REGION="${INFRA_REGION:-us-east-2}"
INFRA_PROFILE="${INFRA_PROFILE:-sphericon}"
INFRA_APP_SECRET="${INFRA_APP_SECRET:-sphericon/app}"

die() {
  echo "error: $*" >&2
  exit 1
}

require_tools() {
  local tool
  for tool in "$@"; do
    command -v "$tool" >/dev/null || die "$tool is not installed (mise install)"
  done
}

# Exports AWS_* credentials of the `aws login` session of the profile (short lived; the CLI
# refreshes them). Nothing is printed.
aws_login() {
  require_tools aws
  local creds
  creds="$(aws configure export-credentials --profile "$INFRA_PROFILE" --format env 2>/dev/null)" ||
    die "no credentials for profile '$INFRA_PROFILE': run 'aws login --profile $INFRA_PROFILE'"
  eval "$creds"
  export AWS_DEFAULT_REGION="$INFRA_REGION" AWS_REGION="$INFRA_REGION" AWS_PAGER=""
  unset AWS_PROFILE
}

# State bucket: one per account, named after it so the name is globally unique and nothing
# account specific is committed.
state_bucket() {
  local account
  account="$(aws sts get-caller-identity --query Account --output text)"
  echo "sphericon-tfstate-$account"
}
