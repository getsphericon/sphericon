// PROTOTYPE (throwaway, ticket #958). Floating variant switcher: arrows and ←/→ keys.
import { ActionIcon, Group, Paper, Text } from '@mantine/core'
import { useEffect } from 'react'
import { useSearchParams } from 'react-router'

export function Switcher({ variants }: { variants: { key: string; name: string }[] }) {
  const [params, setParams] = useSearchParams()
  const current = params.get('variant') ?? variants[0].key
  const index = Math.max(0, variants.findIndex((v) => v.key === current))
  const go = (delta: number) => {
    const next = variants[(index + delta + variants.length) % variants.length]
    setParams((p) => {
      p.set('variant', next.key)
      return p
    })
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) {return}
      if (e.key === 'ArrowLeft') {go(-1)}
      if (e.key === 'ArrowRight') {go(1)}
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  return (
    <Paper
      shadow="xl"
      radius="xl"
      px="md"
      py={6}
      bg="dark.7"
      style={{ position: 'fixed', bottom: 16, left: '50%', transform: 'translateX(-50%)', zIndex: 3000 }}
    >
      <Group gap="sm">
        <ActionIcon variant="subtle" color="gray" onClick={() => go(-1)} aria-label="Previous variant">
          ←
        </ActionIcon>
        <Text c="white" size="sm" fw={600}>
          PROTOTYPE {variants[index].key} ({variants[index].name})
        </Text>
        <ActionIcon variant="subtle" color="gray" onClick={() => go(1)} aria-label="Next variant">
          →
        </ActionIcon>
      </Group>
    </Paper>
  )
}
