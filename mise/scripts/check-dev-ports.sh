#!/usr/bin/env bash
# The dev Caddy listens on CADDY_PORT and CADDY_HTTP_PORT: 443 and 80 in the primary checkout.
# Linux lets an unprivileged process bind a port only from net.ipv4.ip_unprivileged_port_start
# up (1024 by default), so there Caddy fails with `bind: permission denied`, which only its
# daemon log shows. This stops `mise run dev` first and says what to do. macOS has no such limit.
set -euo pipefail

[[ "$(uname -s)" == Linux ]] || exit 0

lowest=$((CADDY_PORT < CADDY_HTTP_PORT ? CADDY_PORT : CADDY_HTTP_PORT))
start="$(cat /proc/sys/net/ipv4/ip_unprivileged_port_start 2>/dev/null || echo 1024)"
((lowest < start)) || exit 0

caddy="$(readlink -f "$(command -v caddy)")"
if command -v getcap >/dev/null && getcap "$caddy" | grep -q cap_net_bind_service; then
  exit 0
fi

cat >&2 <<EOF
error: Caddy cannot listen on ports ${CADDY_PORT} and ${CADDY_HTTP_PORT}: Linux reserves ports below ${start} for root (net.ipv4.ip_unprivileged_port_start).
Either allow them, which keeps ${SPHERICON_APP_URL}:
  sudo sysctl -w net.ipv4.ip_unprivileged_port_start=${lowest}
  echo 'net.ipv4.ip_unprivileged_port_start=${lowest}' | sudo tee /etc/sysctl.d/50-unprivileged-ports.conf
or move Caddy to an unprivileged port with this in .mise.local.toml (the whole block: mise refuses a
daemon without run), then check the URL with mise env | grep SPHERICON_APP_URL:
EOF
# The block as .mise.toml declares it now, up to its first blank line, with only the port changed.
awk '/^\[daemons\.caddy\]$/ { on = 1 } on && !NF { exit } on' .mise.toml |
  sed -E 's/^port = .*/port = 8443/; s/^/  /' >&2
exit 1
