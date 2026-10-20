import { expect, test } from 'vitest'

import { useNotify } from '../hooks/useNotify.ts'
import { renderWithProviders } from '../test/renderWithProviders.tsx'

function SaveButton() {
  const notify = useNotify()
  return (
    <button
      type="button"
      onClick={() => {
        notify({ message: 'Saved', autoClose: false })
      }}
    >
      Save
    </button>
  )
}

test('a toast shown in one render is gone in the next one', async () => {
  const first = await renderWithProviders(<SaveButton />)
  await first.getByRole('button', { name: 'Save' }).click()
  await expect.element(first.getByText('Saved')).toBeInTheDocument()
  await first.unmount()

  const second = await renderWithProviders(<p>Next test</p>)

  await expect.element(second.getByText('Next test')).toBeInTheDocument()
  await expect.element(second.getByText('Saved')).not.toBeInTheDocument()
})
