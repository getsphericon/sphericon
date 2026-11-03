import { Blockquote, Code, Image, Paper, Stack, Text } from '@mantine/core'
import type { ReactNode } from 'react'
import { useMemo } from 'react'

import type { TelegramContentValue } from '../../utils/telegramContent.ts'
import { completeButtons, resolveTags } from '../../utils/telegramContent.ts'

function span(key: string, children: ReactNode[], props: Record<string, unknown>) {
  return (
    <Text key={key} component="span" inherit {...props}>
      {children}
    </Text>
  )
}

// What the preview does with each tag Telegram keeps; the rest is dropped as the server does.
function element(tag: string, key: string, children: ReactNode[]): ReactNode {
  switch (tag) {
    case 'b':
    case 'strong':
      return span(key, children, { fw: 700 })
    case 'i':
    case 'em':
      return span(key, children, { fs: 'italic' })
    case 'u':
    case 'ins':
      return span(key, children, { td: 'underline' })
    case 's':
    case 'strike':
    case 'del':
      return span(key, children, { td: 'line-through' })
    case 'a':
      // A link is shown as one, never followed from the preview.
      return span(key, children, { c: 'blue', td: 'underline' })
    case 'code':
      return <Code key={key}>{children}</Code>
    case 'pre':
      return (
        <Code key={key} block>
          {children}
        </Code>
      )
    case 'blockquote':
      return <Blockquote key={key}>{children}</Blockquote>
    case 'tg-spoiler':
      return span(key, children, { bg: 'gray.7', c: 'gray.7' })
    default:
      return children
  }
}

function textNodes(text: string, key: string): ReactNode[] {
  const out: ReactNode[] = []
  for (const [i, line] of text.split('\n').entries()) {
    if (i > 0) {
      out.push(<br key={`${key}.${i}`} />)
    }
    out.push(line)
  }
  return out
}

function toNodes(parent: Node, prefix: string): ReactNode[] {
  const out: ReactNode[] = []
  for (const [i, node] of [...parent.childNodes].entries()) {
    const key = `${prefix}.${i}`
    if (node.nodeType === Node.TEXT_NODE) {
      out.push(...textNodes(node.textContent ?? '', key))
    } else if (node instanceof Element) {
      const tag = node.tagName.toLowerCase()
      if (tag === 'br') {
        out.push(<br key={key} />)
      } else if (tag !== 'script' && tag !== 'style') {
        out.push(element(tag, key, toNodes(node, key)))
      }
    }
  }
  return out
}

// TelegramPreview approximates the message's bubble for a sample Contact: the merge tags resolved,
// the allowed formatting applied, the image above and the buttons below, one per row.
export function TelegramPreview({ content }: { content: TelegramContentValue }) {
  const resolved = useMemo(() => resolveTags(content.text), [content.text])
  const nodes = useMemo(() => {
    const doc = new DOMParser().parseFromString(`<body>${resolved}</body>`, 'text/html')
    return toNodes(doc.body, 'p')
  }, [resolved])
  const buttons = completeButtons(content.buttons)

  return (
    <Stack gap={4} maw={360} data-testid="telegram-preview">
      <Paper withBorder radius="md" p="sm" bg="blue.0">
        {content.imageUrl.trim() === '' ? null : (
          <Image src={content.imageUrl} alt="" radius="sm" mb="xs" />
        )}
        <Text size="sm">{nodes}</Text>
      </Paper>
      {buttons.map((b, i) => (
        <Paper key={i} withBorder radius="sm" py={4} px="sm" ta="center">
          <Text size="sm" c="blue">
            {b.label}
          </Text>
        </Paper>
      ))}
    </Stack>
  )
}
