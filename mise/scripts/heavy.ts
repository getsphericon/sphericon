// Runs a CPU-heavy command (go test, golangci-lint, tsc, vitest, code generation) while holding one
// of a few machine-wide slots, so many checkouts and agents running tasks at once queue up instead
// of all starting together and starving each other.
//
//   node mise/scripts/heavy.ts <command> [args...]
//
// A slot is a loopback TCP port: the process that has it bound holds the slot, and a second bind
// fails with EADDRINUSE. The kernel frees the port when its holder dies, however it dies, and a
// command the holder starts does not inherit the socket. SPHERICON_HEAVY_SLOTS is how many may run
// at once (default: a quarter of the cores, at least one); the slots are the SPHERICON_HEAVY_PORT
// port (default 47810) and the ones after it. A command started inside a held slot (a task that
// calls another heavy task) runs at once, so nesting never deadlocks. CI has a machine of its own
// and skips the queue.
import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import type { Server } from 'node:net'
import { constants, availableParallelism } from 'node:os'
import { setTimeout as sleep } from 'node:timers/promises'

const [command, ...args] = process.argv.slice(2)
if (command === undefined) {
  process.stderr.write('usage: node mise/scripts/heavy.ts <command> [args...]\n')
  process.exit(2)
}

const slots = Math.max(1, Number(process.env.SPHERICON_HEAVY_SLOTS ?? availableParallelism() >> 2))
const firstPort = Number(process.env.SPHERICON_HEAVY_PORT ?? 47_810)

// The server bound to the port, or undefined when another process holds it.
async function bind(port: number): Promise<Server | undefined> {
  return await new Promise((resolve, reject) => {
    const server = createServer()
    server.once('error', (error: NodeJS.ErrnoException) => {
      if (error.code === 'EADDRINUSE') {
        resolve(undefined)
      } else {
        reject(error)
      }
    })
    server.listen(port, '127.0.0.1', () => {
      resolve(server)
    })
  })
}

// Waits for a free slot and returns the server that holds it.
async function acquire(): Promise<Server> {
  let announced = false
  for (;;) {
    for (let slot = 0; slot < slots; slot++) {
      const server = await bind(firstPort + slot)
      if (server !== undefined) {
        return server
      }
    }
    if (!announced) {
      process.stderr.write(`heavy: all ${slots} slots are busy, waiting for one\n`)
      announced = true
    }
    await sleep(500 + Math.random() * 1000)
  }
}

const queued = process.env.SPHERICON_HEAVY_HELD === undefined && !process.env.CI
const holder = queued ? await acquire() : undefined

const child = spawn(command, args, {
  stdio: 'inherit',
  env: { ...process.env, SPHERICON_HEAVY_HELD: '1' },
})
for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP'] as const) {
  process.on(signal, () => {
    child.kill(signal)
  })
}
child.once('error', (error) => {
  process.stderr.write(`heavy: ${command}: ${error.message}\n`)
  holder?.close()
  process.exit(127)
})
child.once('exit', (code, signal) => {
  holder?.close()
  process.exit(code ?? 128 + (signal === null ? 0 : constants.signals[signal]))
})
