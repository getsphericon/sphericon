// Runtime shims for goja (the split bug is rewritten at build time, see build.ts). Each one is
// feature-detected, so the same bundle also loads in Node (the build runs it there to derive the
// schema listing). goja itself (the Go side, internal/kernel/emaildoc) provides `console`,
// `goUtf8Encode` and `goUtf8Decode`; everything else is here. build.ts compiles this module to a
// classic script and puts it before the bundle, so it runs before any module of the bundle.

import { Base64 } from 'js-base64'
import { ReadableStream, WritableStream } from 'web-streams-polyfill'

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
  Object.assign(globalThis, { atob: Base64.atob })
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

// @react-email/render's browser build reads React's streaming renderer through a ReadableStream
// and pipes it into a WritableStream.
if (!Object.hasOwn(globalThis, 'ReadableStream')) {
  Object.assign(globalThis, { ReadableStream })
}
if (!Object.hasOwn(globalThis, 'WritableStream')) {
  Object.assign(globalThis, { WritableStream })
}
