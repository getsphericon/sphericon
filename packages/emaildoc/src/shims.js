// Runtime shims for goja (the split bug is rewritten at build time, see build.mjs). Each one is feature-detected, so the same bundle also loads in Node (the
// build runs it there to derive the schema listing). goja itself (the Go side, internal/emaildoc)
// provides `console`, `goUtf8Encode` and `goUtf8Decode`; everything else is here.
;(() => {
  const g = globalThis

  // Timers: a queue the host drains (`drainTimers`) while it waits for the render's promise.
  if (!Object.hasOwn(g, 'setTimeout')) {
    let timers = []
    let nextId = 1
    g.setTimeout = (fn, _delay, ...args) => {
      const id = nextId++
      timers.push({
        id,
        run: () => {
          fn(...args)
        },
      })
      return id
    }
    g.setImmediate = (fn, ...args) => g.setTimeout(fn, 0, ...args)
    g.clearTimeout = (id) => {
      timers = timers.filter((timer) => timer.id !== id)
    }
    g.clearImmediate = g.clearTimeout
    // Runs the oldest timer; false when there is none.
    g.drainTimers = () => {
      const next = timers.shift()
      if (next === undefined) {
        return false
      }
      next.run()
      return true
    }
  }

  // UTF-8 in Go: a JavaScript version dominated the cost of big documents.
  if (!Object.hasOwn(g, 'TextEncoder')) {
    g.TextEncoder = class TextEncoder {
      encoding = 'utf-8'

      encode(text = '') {
        return new Uint8Array(g.goUtf8Encode(text))
      }
    }
  }
  if (!Object.hasOwn(g, 'TextDecoder')) {
    g.TextDecoder = class TextDecoder {
      encoding = 'utf-8'

      decode(bytes) {
        if (bytes === undefined) {
          return ''
        }
        const view = ArrayBuffer.isView(bytes)
          ? new Uint8Array(bytes.buffer, bytes.byteOffset, bytes.byteLength)
          : new Uint8Array(bytes)
        return g.goUtf8Decode(view.buffer.slice(view.byteOffset, view.byteOffset + view.byteLength))
      }
    }
  }

  // react-dom/server's browser build creates a MessageChannel at module init.
  if (!Object.hasOwn(g, 'MessageChannel')) {
    g.MessageChannel = class MessageChannel {
      port1 = { onmessage: null }

      port2 = {
        postMessage: (message) => {
          g.setTimeout(() => {
            this.port1.onmessage?.({ data: message })
          }, 0)
        },
      }
    }
  }

  // @react-email/render's browser build reads React's streaming renderer through a ReadableStream
  // and pipes it into a WritableStream. This is the part of the two that it uses.
  if (!Object.hasOwn(g, 'ReadableStream')) {
    g.ReadableStream = class ReadableStream {
      #queue = []
      #waiting = []
      #closed = false
      #failed = false
      #failure
      #source
      #started = false
      #controller = {
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

      constructor(source) {
        this.#source = source
      }

      #read() {
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
          read: () => {
            const ready = this.#read()
            if (ready !== null) {
              return ready
            }
            this.#source?.pull?.(this.#controller)
            return (
              this.#read() ??
              new Promise((resolve, reject) => {
                this.#waiting.push({ resolve, reject })
              })
            )
          },
          releaseLock() {},
          cancel: () => Promise.resolve(),
        }
      }

      async pipeTo(destination) {
        const reader = this.getReader()
        const writer = destination.getWriter()
        for (;;) {
          const step = await reader.read()
          if (step.done) {
            return writer.close()
          }
          await writer.write(step.value)
        }
      }
    }
  }
  if (!Object.hasOwn(g, 'WritableStream')) {
    g.WritableStream = class WritableStream {
      #sink

      constructor(sink) {
        this.#sink = sink
      }

      getWriter() {
        const sink = this.#sink
        return {
          write: (chunk) => Promise.resolve(sink?.write?.(chunk)),
          close: () => Promise.resolve(sink?.close?.()),
          abort: (reason) => Promise.resolve(sink?.abort?.(reason)),
          releaseLock() {},
        }
      }
    }
  }
})()
