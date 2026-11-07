// Runtime shims for goja (the split bug is rewritten at build time, see build.ts). Each one is
// feature-detected, so the same bundle also loads in Node (the build runs it there to derive the
// schema listing). goja itself (the Go side, internal/kernel/emaildoc) provides `console`,
// `goUtf8Encode` and `goUtf8Decode`; everything else is here. build.ts compiles this module to a
// classic script and puts it before the bundle, so it runs before any module of the bundle.

/** What goja provides beside the standard globals (internal/kernel/emaildoc sets it). */
export interface GoHost {
  goUtf8Encode: (text: string) => ArrayBuffer
  goUtf8Decode: (bytes: ArrayBufferLike) => string
}

function isGoHost(host: object): host is GoHost {
  return (
    'goUtf8Encode' in host &&
    typeof host.goUtf8Encode === 'function' &&
    'goUtf8Decode' in host &&
    typeof host.goUtf8Decode === 'function'
  )
}

// The UTF-8 shims stand in only where goja runs the bundle, which provides these two.
function goHost(): GoHost {
  const host: object = globalThis
  if (!isGoHost(host)) {
    throw new Error('the host provides no goUtf8Encode and goUtf8Decode')
  }
  return host
}

// Timers: a queue the host drains (`drainTimers`) while it waits for the render's promise.
if (!Object.hasOwn(globalThis, 'setTimeout')) {
  let timers: { id: number; run: () => void }[] = []
  let nextId = 1
  const queueTimer = (fn: (...args: unknown[]) => void, _delay?: number, ...args: unknown[]) => {
    const id = nextId++
    timers.push({
      id,
      run: () => {
        fn(...args)
      },
    })
    return id
  }
  const cancelTimer = (id: number) => {
    timers = timers.filter((timer) => timer.id !== id)
  }
  Object.assign(globalThis, {
    setTimeout: queueTimer,
    setImmediate: (fn: (...args: unknown[]) => void, ...args: unknown[]) =>
      queueTimer(fn, 0, ...args),
    clearTimeout: cancelTimer,
    clearImmediate: cancelTimer,
    // Runs the oldest timer; false when there is none.
    drainTimers: () => {
      const next = timers.shift()
      if (next === undefined) {
        return false
      }
      next.run()
      return true
    },
  })
}

// UTF-8 in Go: a JavaScript version dominated the cost of big documents.
if (!Object.hasOwn(globalThis, 'TextEncoder')) {
  Object.assign(globalThis, {
    TextEncoder: class TextEncoder {
      encoding = 'utf-8'

      encode(text = '') {
        return new Uint8Array(goHost().goUtf8Encode(text))
      }
    },
  })
}
if (!Object.hasOwn(globalThis, 'TextDecoder')) {
  Object.assign(globalThis, {
    TextDecoder: class TextDecoder {
      encoding = 'utf-8'

      decode(bytes?: ArrayBufferLike | ArrayBufferView) {
        if (bytes === undefined) {
          return ''
        }
        const view = ArrayBuffer.isView(bytes)
          ? new Uint8Array(bytes.buffer, bytes.byteOffset, bytes.byteLength)
          : new Uint8Array(bytes)
        return goHost().goUtf8Decode(
          view.buffer.slice(view.byteOffset, view.byteOffset + view.byteLength),
        )
      }
    },
  })
}

// entities 7 (htmlparser2, under @react-email/render's html-to-text) decodes its entity tables
// from base64 at module init, through `atob` where there is one and Node's `Buffer` otherwise.
if (!Object.hasOwn(globalThis, 'atob')) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
  Object.assign(globalThis, {
    atob: (data: string) => {
      const input = data.replace(/[\t\n\f\r =]/g, '')
      const codes: number[] = []
      let buffer = 0
      let bits = 0
      for (const char of input) {
        const value = alphabet.indexOf(char)
        if (value < 0) {
          throw new Error('atob: the string is not correctly encoded')
        }
        buffer = (buffer << 6) | value
        bits += 6
        if (bits >= 8) {
          bits -= 8
          codes.push((buffer >> bits) & 0xff)
          buffer &= (1 << bits) - 1
        }
      }
      let output = ''
      for (let i = 0; i < codes.length; i += 8192) {
        output += String.fromCharCode(...codes.slice(i, i + 8192))
      }
      return output
    },
  })
}

// react-dom/server's browser build creates a MessageChannel at module init.
if (!Object.hasOwn(globalThis, 'MessageChannel')) {
  Object.assign(globalThis, {
    MessageChannel: class MessageChannel {
      port1: { onmessage: ((event: { data: unknown }) => void) | null } = { onmessage: null }

      port2 = {
        postMessage: (message: unknown) => {
          setTimeout(() => {
            this.port1.onmessage?.({ data: message })
          }, 0)
        },
      }
    },
  })
}

/** One step of a read: a chunk, or the end of the stream. */
interface Step {
  value: unknown
  done: boolean
}

/** The controller a stream's source writes through. */
interface Controller {
  enqueue: (chunk: unknown) => void
  close: () => void
  error: (error: unknown) => void
}

/** The source a ReadableStream is made from. */
interface Source {
  start?: (controller: Controller) => void
  pull?: (controller: Controller) => void
}

/** The sink a WritableStream writes to. */
interface Sink {
  write?: (chunk: unknown) => unknown
  close?: () => unknown
  abort?: (reason: unknown) => unknown
}

/** The writer of a WritableStream. */
interface Writer {
  write: (chunk: unknown) => Promise<unknown>
  close: () => Promise<unknown>
  abort: (reason: unknown) => Promise<unknown>
  releaseLock: () => void
}

// @react-email/render's browser build reads React's streaming renderer through a ReadableStream
// and pipes it into a WritableStream. This is the part of the two that it uses.
if (!Object.hasOwn(globalThis, 'ReadableStream')) {
  Object.assign(globalThis, {
    ReadableStream: class ReadableStream {
      readonly #queue: unknown[] = []
      readonly #waiting: { resolve: (step: Step) => void; reject: (error: unknown) => void }[] = []
      #closed = false
      #failed = false
      #failure: unknown
      readonly #source: Source | undefined
      #started = false
      readonly #controller: Controller = {
        enqueue: (chunk) => {
          const reader = this.#waiting.shift()
          if (reader === undefined) {
            this.#queue.push(chunk)
          } else {
            reader.resolve({ value: chunk, done: false })
          }
        },
        close: () => {
          this.#closed = true
          for (const reader of this.#waiting.splice(0)) {
            reader.resolve({ value: undefined, done: true })
          }
        },
        error: (error) => {
          this.#failed = true
          this.#failure = error
          for (const reader of this.#waiting.splice(0)) {
            reader.reject(error)
          }
        },
      }

      constructor(source?: Source) {
        this.#source = source
      }

      #read(): Promise<Step> | null {
        if (this.#queue.length > 0) {
          return Promise.resolve({ value: this.#queue.shift(), done: false })
        }
        if (this.#failed) {
          return Promise.reject(this.#failure)
        }
        if (this.#closed) {
          return Promise.resolve({ value: undefined, done: true })
        }
        return null
      }

      getReader() {
        if (!this.#started) {
          this.#started = true
          this.#source?.start?.(this.#controller)
        }
        return {
          read: async (): Promise<Step> => {
            const ready = this.#read()
            if (ready !== null) {
              return ready
            }
            this.#source?.pull?.(this.#controller)
            return (
              this.#read() ??
              new Promise<Step>((resolve, reject) => {
                this.#waiting.push({ resolve, reject })
              })
            )
          },
          releaseLock() {
            // Nothing is locked: the shim reads from its own queue.
          },
          cancel: async () => Promise.resolve(),
        }
      }

      async pipeTo(destination: { getWriter: () => Writer }): Promise<unknown> {
        const reader = this.getReader()
        const writer = destination.getWriter()
        // One chunk at a time, in order: the next is read once the last is written.
        const pump = async (): Promise<unknown> => {
          const step = await reader.read()
          if (step.done) {
            return writer.close()
          }
          await writer.write(step.value)
          return pump()
        }
        return pump()
      }
    },
  })
}
if (!Object.hasOwn(globalThis, 'WritableStream')) {
  Object.assign(globalThis, {
    WritableStream: class WritableStream {
      readonly #sink: Sink | undefined

      constructor(sink?: Sink) {
        this.#sink = sink
      }

      getWriter(): Writer {
        const sink = this.#sink
        return {
          write: async (chunk) => Promise.resolve(sink?.write?.(chunk)),
          close: async () => Promise.resolve(sink?.close?.()),
          abort: async (reason) => Promise.resolve(sink?.abort?.(reason)),
          releaseLock() {
            // Nothing is locked: the shim writes straight to its sink.
          },
        }
      }
    },
  })
}
