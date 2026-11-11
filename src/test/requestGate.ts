// A gate a mocked handler holds its first request at: the test learns when the request arrived,
// acts while it is pending, then releases it. Later requests pass. `answered` resolves once the
// held handler has returned its response.
export function requestGate() {
  const received = Promise.withResolvers<undefined>()
  const release = Promise.withResolvers<undefined>()
  const answered = Promise.withResolvers<undefined>()
  let held = false
  return {
    received: received.promise,
    release: () => {
      release.resolve(undefined)
    },
    answered: answered.promise,
    // Awaited by the handler before it answers.
    hold: async () => {
      if (held) {
        return
      }
      held = true
      received.resolve(undefined)
      await release.promise
      queueMicrotask(() => {
        answered.resolve(undefined)
      })
    },
  }
}
