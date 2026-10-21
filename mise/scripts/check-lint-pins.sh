#!/usr/bin/env bash
# The analyzers (tools/analyzers) are compiled into golangci-lint, so the dependencies they share
# with it must be the versions golangci-lint was built with, or the custom build fails or misbehaves.
# This checks that .custom-gcl.yml names the golangci-lint release pinned in mise.lock, and that
# tools/analyzers/go.mod requires that release's golang.org/x/tools and plugin-module-register.
set -euo pipefail

want="$(grep -A3 '^\[\[tools.golangci-lint\]\]' mise.lock | sed -n 's/^version = "\(.*\)"/\1/p')"
have="$(sed -n 's/^version: v\(.*\)/\1/p' .custom-gcl.yml)"
if [[ -z "$want" || "$want" != "$have" ]]; then
  echo "error: .custom-gcl.yml builds golangci-lint v${have:-?} but mise.lock pins ${want:-?}; set 'version: v${want}' in .custom-gcl.yml (then mise run build:lint)" >&2
  exit 1
fi

gomod="$(go mod download -json "github.com/golangci/golangci-lint/v2@v${want}" | jq -r .GoMod)"
status=0
for mod in golang.org/x/tools github.com/golangci/plugin-module-register; do
  lint_v="$(awk -v m="$mod" '$1 == m { print $2 }' "$gomod" | head -1)"
  ours="$(cd tools/analyzers && go list -m -f '{{.Version}}' "$mod")"
  if [[ "$lint_v" != "$ours" ]]; then
    echo "error: tools/analyzers requires $mod $ours but golangci-lint v${want} uses $lint_v; run 'cd tools/analyzers && go get $mod@$lint_v && go mod tidy'" >&2
    status=1
  fi
done
exit "$status"
