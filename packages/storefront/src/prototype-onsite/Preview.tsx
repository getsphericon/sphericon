// PROTOTYPE (throwaway, ticket #958). The builder's preview calls the same renderer the site does.
import { Box, Group, SegmentedControl, Stack, Text } from '@mantine/core'
import { useEffect, useRef, useState } from 'react'

import type { Device, FormDoc } from './doc.ts'
import { renderForm } from './render.ts'

export function Preview({ doc, device, title = 'Shipped renderer (plain DOM, shadow root)' }: { doc: FormDoc; device: Device; title?: string }) {
  const ref = useRef<HTMLDivElement>(null)
  const steps = [...doc.steps.map((_, i) => String(i)), ...(doc.success ? ['success'] : []), ...(doc.teaser ? ['teaser'] : [])]
  const [start, setStart] = useState('0')
  const startValue = steps.includes(start) ? start : '0'

  useEffect(() => {
    const node = ref.current
    if (!node) {return}
    node.replaceChildren()
    const s = startValue === 'success' || startValue === 'teaser' ? startValue : Number(startValue)
    const r = renderForm(doc, { device, mode: 'preview', container: node, start: s })
    return () => r.destroy()
  }, [doc, device, startValue])

  return (
    <Stack gap={6}>
      <Group justify="space-between">
        <Text size="xs" c="dimmed">
          {title}
        </Text>
        <SegmentedControl
          size="xs"
          value={startValue}
          onChange={setStart}
          data={steps.map((s) => ({ value: s, label: /^\d+$/.test(s) ? `Step ${Number(s) + 1}` : s }))}
        />
      </Group>
      <Box
        ref={ref}
        style={{
          width: device === 'mobile' ? 375 : '100%',
          margin: '0 auto',
          border: '1px dashed var(--mantine-color-gray-4)',
          borderRadius: 8,
          padding: 12,
          minHeight: 120,
        }}
      />
    </Stack>
  )
}
