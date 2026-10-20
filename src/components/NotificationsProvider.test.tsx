import { useEffect } from 'react'
import { expect, test } from 'vitest'

import { useNotify } from '../hooks/useNotify.ts'
import { renderWithProviders } from '../test/renderWithProviders.tsx'

function ShowsAToast() {
  const notify = useNotify()
  useEffect(() => {
    notify({ message: 'Saved', autoClose: false })
  }, [notify])
  return null
}

test('a toast shown in one render is gone in the next one', async () => {
  const first = await renderWithProviders(<ShowsAToast />)
  await expect.element(first.getByText('Saved')).toBeInTheDocument()
  await first.unmount()

  // The test id, not a label: the i18n lint flags hardcoded text in JSX.
  const second = await renderWithProviders(<div data-testid="next" />)

  await expect.element(second.getByTestId('next')).toBeInTheDocument()
  await expect.element(second.getByText('Saved')).not.toBeInTheDocument()
})
