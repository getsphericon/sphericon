import { expect, test } from 'vitest'

import { renderWithProviders } from '../../test/renderWithProviders.tsx'
import { TelegramPreview } from './TelegramPreview.tsx'

const BASE = { text: '', imageUrl: '', buttons: [], linkPreview: true }

test('the allowed formatting is applied and a merge tag shows the sample Contact', async () => {
  const screen = await renderWithProviders(
    <TelegramPreview
      content={{
        ...BASE,
        text:
          '<b>Hi</b> <i>{{ first_name }}</i> <u>u</u> <s>s</s> <a href="https://x.test">link</a>\n' +
          '<code>c</code><pre>p</pre><blockquote>q</blockquote><tg-spoiler>sp</tg-spoiler>' +
          '<em>e</em><br>after<script>alert(1)</script>',
      }}
    />,
  )

  const bubble = screen.getByTestId('telegram-preview')
  await expect.element(bubble).toHaveTextContent('Hi Alex u s linkcpqspeafter')
})

test('the image sits above the text and complete buttons below, one per row', async () => {
  const screen = await renderWithProviders(
    <TelegramPreview
      content={{
        ...BASE,
        text: 'Hello',
        imageUrl: 'https://cdn.example.com/a.png',
        buttons: [
          { label: 'Open', url: 'https://example.com/a' },
          { label: '', url: 'https://example.com/b' },
        ],
      }}
    />,
  )

  const bubble = screen.getByTestId('telegram-preview')
  expect(screen.container.querySelector('img')?.getAttribute('src')).toBe(
    'https://cdn.example.com/a.png',
  )
  await expect.element(bubble).toHaveTextContent('HelloOpen')
})
