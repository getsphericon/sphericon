// The dev Caddy listens on CADDY_PORT and CADDY_HTTP_PORT: 443 and 80 in the primary checkout.
// Linux lets an unprivileged process bind a port only from net.ipv4.ip_unprivileged_port_start
// up (1024 by default), so there Caddy fails with `bind: permission denied`, which only its
// daemon log shows. This stops `mise run dev` first and says what to do. macOS has no such limit.
import { execFileSync } from 'node:child_process'
import { readFileSync, realpathSync } from 'node:fs'

if (process.platform !== 'linux') {
  process.exit(0)
}

const caddyPort = Number(process.env.CADDY_PORT)
const caddyHttpPort = Number(process.env.CADDY_HTTP_PORT)
const lowest = Math.min(caddyPort, caddyHttpPort)

function unprivilegedStart(): number {
  try {
    return Number(readFileSync('/proc/sys/net/ipv4/ip_unprivileged_port_start', 'utf8').trim())
  } catch {
    return 1024
  }
}

function output(command: string, args: string[]): string {
  try {
    return execFileSync(command, args, {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim()
  } catch {
    return ''
  }
}

const start = unprivilegedStart()
if (lowest >= start) {
  process.exit(0)
}

const caddyPath = output('which', ['caddy'])
if (
  caddyPath !== '' &&
  output('getcap', [realpathSync(caddyPath)]).includes('cap_net_bind_service')
) {
  process.exit(0)
}

// The block as .mise.toml declares it now, up to its first blank line, with only the port changed.
const block: string[] = []
let inBlock = false
for (const line of readFileSync('.mise.toml', 'utf8').split('\n')) {
  if (line === '[daemons.caddy]') {
    inBlock = true
  }
  if (inBlock && line.trim() === '') {
    break
  }
  if (inBlock) {
    block.push(`  ${line.replace(/^port = .*/, 'port = 8443')}`)
  }
}

process.stderr.write(
  [
    `error: Caddy cannot listen on ports ${caddyPort} and ${caddyHttpPort}: Linux reserves ports below ${start} for root (net.ipv4.ip_unprivileged_port_start).`,
    `Either allow them, which keeps ${process.env.SPHERICON_APP_URL}:`,
    `  sudo sysctl -w net.ipv4.ip_unprivileged_port_start=${lowest}`,
    `  echo 'net.ipv4.ip_unprivileged_port_start=${lowest}' | sudo tee /etc/sysctl.d/50-unprivileged-ports.conf`,
    'or move Caddy to an unprivileged port with this in .mise.local.toml (the whole block: mise refuses a',
    'daemon without run), then check the URL with mise env | grep SPHERICON_APP_URL:',
    ...block,
    '',
  ].join('\n'),
)
process.exit(1)
