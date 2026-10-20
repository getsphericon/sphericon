#!/usr/bin/env bash
# Run schemalens against the migrated dev database with its ent adapter. The adapter ships only as
# a release archive (not go-installable, and mise's github backend installs one asset per repo), so
# it is fetched once per pinned version into the user cache, checked against the release checksums.
set -euo pipefail

version="${SCHEMALENS_VERSION:?set by the mise tools table}"
cache="${XDG_CACHE_HOME:-$HOME/.cache}/sphericon/schemalens-adapter-ent-$version"

if [ ! -x "$cache/schemalens-adapter-ent" ]; then
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  arch=$(uname -m)
  case "$arch" in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; esac
  archive="schemalens-adapter-ent_${version}_${os}_${arch}.tar.gz"
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  gh release download "v$version" -R mokevnin/schemalens -p "$archive" -p checksums.txt -D "$tmp"
  (cd "$tmp" && grep " $archive\$" checksums.txt | shasum -a 256 -c -)
  mkdir -p "$cache"
  tar -xzf "$tmp/$archive" -C "$cache" schemalens-adapter-ent
fi

PATH="$cache:$PATH" exec schemalens check --dsn "$DATABASE_URL" --adapter ent "$@"
