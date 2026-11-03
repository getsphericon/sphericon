// PROTOTYPE (throwaway, ticket #958). A shop article with a reserved Placement, and the debug panel
// that runs the isolation checks the runtime research left open.
import { Badge, Button, Checkbox, Code, Group, Paper, ScrollArea, Stack, Text, Title } from '@mantine/core'
import { useEffect, useState } from 'react'

import { resetFormState, setHostileCss } from './runtime.ts'

export function ArticlePage() {
  return (
    <Stack>
      <Title order={1}>How to study for 20 minutes a day</Title>
      <Text>
        Short, regular sessions beat long irregular ones. Pick a fixed time, close every other tab, and stop when the timer rings, even in the middle
        of a lesson: the half-finished thought brings you back tomorrow.
      </Text>
      <Text>Below is a Placement the site reserves: the embedded form fills it.</Text>
      {/* The site names a place, not a form (ADR 0085). */}
      <div data-sphericon-placement="article-footer" />
      <Text c="dimmed" size="sm">
        Autofill check: focus the email field inside the form and see whether the browser offers saved addresses.
      </Text>
      <Paper withBorder p="sm">
        <Text size="sm" fw={600}>A plain page form, outside any shadow root, for comparison</Text>
        <input type="email" name="email" autoComplete="email" placeholder="you@example.com" />
      </Paper>
    </Stack>
  )
}

export function DebugPanel({ log, forceMobile, setForceMobile }: { log: string[]; forceMobile: boolean; setForceMobile: (v: boolean) => void }) {
  const [hostile, setHostile] = useState(() => sessionStorage.getItem('PROTOTYPE_hostile') === '1')
  const strict = new URLSearchParams(location.search).get('csp') === 'strict'
  const [inert, setInert] = useState('')
  useEffect(() => {
    setHostileCss(hostile)
    sessionStorage.setItem('PROTOTYPE_hostile', hostile ? '1' : '0')
  }, [hostile])
  useEffect(() => {
    const t = window.setInterval(() => {
      const open = document.querySelector('[data-sphericon-form]')?.shadowRoot?.querySelector('dialog[open]')
      // A modal <dialog> makes the rest of its document inert: is the shop's header button still focusable?
      setInert(open ? (document.querySelector('header a, header button'))?.matches(':focus') ? 'page focusable' : 'modal open: try Tab, the page should be skipped' : '')
    }, 500)
    return () => window.clearInterval(t)
  }, [])

  return (
    <Paper shadow="lg" withBorder p="xs" style={{ position: 'fixed', right: 12, bottom: 12, width: 340, zIndex: 3000, fontFamily: 'system-ui', fontSize: 12 }}>
      <Group justify="space-between">
        <Text size="xs" fw={700} c="red">PROTOTYPE debug</Text>
        <Badge size="xs" color={strict ? 'red' : 'gray'}>{strict ? "CSP style-src 'self'" : 'no CSP'}</Badge>
      </Group>
      <Stack gap={4} mt={4}>
        <Checkbox size="xs" label="Hostile site CSS (62.5% root, Comic Sans, global button/input)" checked={hostile} onChange={(e) => setHostile(e.currentTarget.checked)} />
        <Checkbox size="xs" label="Force phone layout" checked={forceMobile} onChange={(e) => setForceMobile(e.currentTarget.checked)} />
        <Group gap={4}>
          <Button size="compact-xs" variant="light" component="a" href={strict ? location.pathname : `${location.pathname}?csp=strict`}>
            {strict ? 'Reload without CSP' : 'Reload with strict CSP'}
          </Button>
          <Button size="compact-xs" variant="light" onClick={() => { resetFormState(); location.reload() }}>Show forms again</Button>
          <Button size="compact-xs" variant="light" component="a" href="/prototype/onsite-builder">Builder</Button>
        </Group>
        {inert && <Text size="xs">{inert}</Text>}
        <ScrollArea h={110}>
          <Code block style={{ fontSize: 10 }}>{log.join('\n') || 'events appear here'}</Code>
        </ScrollArea>
      </Stack>
    </Paper>
  )
}
