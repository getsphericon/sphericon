import type { QueryClient } from '@tanstack/react-query'
import { vi } from 'vitest'

// The QueryClients rendered by the current test, so the end of a test can wait for what the page
// started on its own: a change finishes after the test's last assertion, and the page then reads
// the data again, which must still be answered by the handlers of that test.
const rendered = new Set<QueryClient>()

export function trackQueryClient(client: QueryClient) {
  rendered.add(client)
}

function fetching() {
  return [...rendered].flatMap((client) =>
    client
      .getQueryCache()
      .getAll()
      .filter((query) => query.state.fetchStatus === 'fetching'),
  )
}

function mutating() {
  return [...rendered].reduce((count, client) => count + client.isMutating(), 0)
}

// settled resolves once every change has finished and every read it set off has too. A read
// already running when the test ended (one a loading-state test leaves pending on purpose) is not
// waited for, and a change that never ends is given up on after a bound.
export async function settled() {
  const left = new Set(fetching())
  try {
    await vi.waitFor(
      () => {
        if (mutating() > 0 || fetching().some((query) => !left.has(query))) {
          throw new Error('the page is still busy')
        }
      },
      { timeout: 3000, interval: 10 },
    )
  } catch {
    // Something is pending on purpose; the unmocked-request check below still applies.
  }
  rendered.clear()
}
