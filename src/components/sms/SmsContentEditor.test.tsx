import { useState } from 'react'
import { expect, test } from 'vitest'

import { renderWithProviders } from '../../test/renderWithProviders.tsx'
import { SmsContentEditor } from './SmsContentEditor.tsx'

function Harness({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial)
  return <SmsContentEditor value={value} onChange={setValue} />
}

test('typing a Cyrillic character switches the counter to UCS-2', async () => {
  const screen = await renderWithProviders(<Harness initial="" />)
  const text = screen.getByRole('textbox', { name: 'Message' })

  await text.fill('Hi there')
  await expect.element(screen.getByText('8 characters · GSM-7 · 1 segment')).toBeVisible()

  await text.fill('Hi there, Борис')

  await expect.element(screen.getByText('15 characters · UCS-2 · 1 segment')).toBeVisible()
})

test('a text past one part counts its segments, an extension character as two', async () => {
  const screen = await renderWithProviders(<Harness initial={`${'a'.repeat(159)}€`} />)

  await expect.element(screen.getByText('161 characters · GSM-7 · 2 segments')).toBeVisible()
})
