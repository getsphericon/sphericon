#!/usr/bin/env bash
# One-time, idempotent bootstrap of the hosted deployment (ADR 0028). Creates, only when missing:
#  - the private S3 state bucket (versioned, encrypted, public access blocked, TLS only), and
#  - the Secrets Manager secret `sphericon/app` with JWT_SECRET, ENCRYPTION_KEY, BOOTSTRAP_TOKEN
#    and an empty LICENSE_KEY. An existing secret is never changed, so ENCRYPTION_KEY is never
#    rotated. The values are generated here and never printed.
# Bucket settings are re-applied on every run (they are idempotent).
set -euo pipefail
set +x
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=infra-common.sh
source "$here/infra-common.sh"

require_tools jq go
aws_login
bucket="$(state_bucket)"

if aws s3api head-bucket --bucket "$bucket" >/dev/null 2>&1; then
  echo "bucket '$bucket' exists"
else
  echo "creating private bucket '$bucket' in $INFRA_REGION"
  aws s3api create-bucket --bucket "$bucket" \
    --create-bucket-configuration "LocationConstraint=$INFRA_REGION" >/dev/null
fi
aws s3api put-public-access-block --bucket "$bucket" --public-access-block-configuration \
  BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true
aws s3api put-bucket-versioning --bucket "$bucket" --versioning-configuration Status=Enabled
aws s3api put-bucket-encryption --bucket "$bucket" --server-side-encryption-configuration \
  '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'
aws s3api put-bucket-lifecycle-configuration --bucket "$bucket" --lifecycle-configuration \
  '{"Rules":[{"ID":"expire-old-state-versions","Status":"Enabled","Filter":{},"NoncurrentVersionExpiration":{"NoncurrentDays":90}}]}'
policy="$(jq -nc --arg arn "arn:aws:s3:::$bucket" '{Version:"2012-10-17",Statement:[{Sid:"TLSOnly",Effect:"Deny",Principal:"*",Action:"s3:*",Resource:[$arn,($arn+"/*")],Condition:{Bool:{"aws:SecureTransport":"false"}}}]}')"
aws s3api put-bucket-policy --bucket "$bucket" --policy "$policy"

if aws secretsmanager describe-secret --secret-id "$INFRA_APP_SECRET" >/dev/null 2>&1; then
  echo "secret '$INFRA_APP_SECRET' exists: keeping it"
else
  echo "creating secret '$INFRA_APP_SECRET'"
  random() { aws secretsmanager get-random-password --password-length 64 --exclude-punctuation --query RandomPassword --output text; }
  key="$(cd "$here/../.." && go run ./cmd/server genkey)"
  # The value goes through stdin, not argv.
  jq -nc --arg jwt "$(random)" --arg enc "$key" --arg boot "$(random)" \
    '{JWT_SECRET:$jwt,ENCRYPTION_KEY:$enc,BOOTSTRAP_TOKEN:$boot,LICENSE_KEY:""}' |
    aws secretsmanager create-secret --name "$INFRA_APP_SECRET" \
      --description "Sphericon app secrets, created once by infra:bootstrap" \
      --secret-string file:///dev/stdin >/dev/null
  unset key
fi

cat <<MSG
Done. Optional: set the EE license key with 'mise run infra:license'.
Next: mise run infra -- plan
MSG
