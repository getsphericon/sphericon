import { useSuspenseQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { expect, test } from 'vitest'

import { renderWithProviders } from '../test/renderWithProviders.tsx'
import { QueryBoundary } from './QueryBoundary.tsx'

// A load that fails the first time it is asked and succeeds after that.
function failingOnce() {
  let calls = 0
  return {
    calls: () => calls,
    load: async () => {
      calls += 1
      // Settled a tick later, as a request is.
      await Promise.resolve()
      if (calls === 1) {
        throw new Error('boom')
      }
      return 'loaded'
    },
  }
}

function Loaded({ load }: { load: () => Promise<string> }) {
  const { data } = useSuspenseQuery({ queryKey: ['boundary-test'], queryFn: load })
  return <p>{data}</p>
}

function Toggled({ load }: { load: () => Promise<string> }) {
  const [shown, setShown] = useState(true)
  return (
    <>
      <button type="button" onClick={() => setShown((value) => !value)}>
        {'Toggle'}
      </button>
      {shown ? (
        <QueryBoundary fallback={<p>{'Loading'}</p>} errorTitle="Failed to load">
          <Loaded load={load} />
        </QueryBoundary>
      ) : null}
    </>
  )
}

test('a failed load shows the alert, and the retry fetches it again', async () => {
  const source = failingOnce()
  const screen = await renderWithProviders(<Toggled load={source.load} />)

  await expect.element(screen.getByText('Failed to load').first()).toBeInTheDocument()
  await screen.getByRole('button', { name: 'Try again' }).click()

  await expect.element(screen.getByText('loaded')).toBeInTheDocument()
  expect(source.calls()).toBe(2)
})

test('a failure still cached is fetched again when the boundary is shown again', async () => {
  const source = failingOnce()
  const screen = await renderWithProviders(<Toggled load={source.load} />)
  await expect.element(screen.getByText('Failed to load').first()).toBeInTheDocument()

  await screen.getByRole('button', { name: 'Toggle' }).click()
  await screen.getByRole('button', { name: 'Toggle' }).click()

  await expect.element(screen.getByText('loaded')).toBeInTheDocument()
  expect(source.calls()).toBe(2)
})
