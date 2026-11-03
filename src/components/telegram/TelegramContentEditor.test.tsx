import { useState } from 'react'
import { expect, test } from 'vitest'

import { renderWithProviders } from '../../test/renderWithProviders.tsx'
import type { TelegramContentValue } from '../../utils/telegramContent.ts'
import { TelegramContentEditor } from './TelegramContentEditor.tsx'

const BASE: TelegramContentValue = { text: '', imageUrl: '', buttons: [], linkPreview: true }

function Harness({ initial }: { initial: TelegramContentValue }) {
  const [value, setValue] = useState(initial)
  return <TelegramContentEditor value={value} onChange={setValue} />
}

test('a text over the limit is refused as a message, and under a smaller caption limit with an image', async () => {
  const long = 'x'.repeat(1100)

  const asText = await renderWithProviders(<Harness initial={{ ...BASE, text: long }} />)
  await expect.element(asText.getByText('1,100 / 4,096')).toBeVisible()
  await expect.element(asText.getByText(/in a message/)).not.toBeInTheDocument()
  await asText.unmount()

  const asCaption = await renderWithProviders(
    <Harness initial={{ ...BASE, text: long, imageUrl: 'https://cdn.example.com/a.png' }} />,
  )
  await expect.element(asCaption.getByText('1,100 / 1,024')).toBeVisible()
  await expect.element(asCaption.getByText(/in a caption/)).toBeVisible()
})

test('link buttons are added, edited and removed', async () => {
  const screen = await renderWithProviders(<Harness initial={BASE} />)

  await screen.getByRole('button', { name: 'Add a button' }).click()
  await screen.getByLabelText('Button 1 label').fill('Open')
  await screen.getByLabelText('Button 1 URL').fill('https://example.com/a')
  await expect.element(screen.getByLabelText('Button 1 label')).toHaveValue('Open')

  await screen.getByRole('button', { name: 'Remove button 1' }).click()

  await expect.element(screen.getByLabelText('Button 1 label')).not.toBeInTheDocument()
})

test('a merge tag is inserted into the text and the image URL is edited', async () => {
  const screen = await renderWithProviders(<Harness initial={BASE} />)

  await screen.getByText('Insert a merge tag').click()
  await screen.getByRole('menuitem', { name: 'first_name' }).click()
  await screen.getByLabelText('Image URL').fill('https://cdn.example.com/a.png')

  await expect.element(screen.getByLabelText('Message')).toHaveValue('{{ first_name }}')
  await expect
    .element(screen.getByLabelText('Image URL'))
    .toHaveValue('https://cdn.example.com/a.png')
})

test('the link-preview switch is toggled', async () => {
  const screen = await renderWithProviders(<Harness initial={BASE} />)

  await screen.getByLabelText('Show a preview of links in the text').click()

  await expect
    .element(screen.getByLabelText('Show a preview of links in the text'))
    .not.toBeChecked()
})
