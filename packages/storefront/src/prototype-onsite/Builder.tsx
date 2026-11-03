// PROTOTYPE (throwaway, branch prototype/onsite-form-builder, ticket #958).
// "Three On-site form builders on /prototype/onsite-builder, switchable via ?variant=A|B|C":
//   A: Puck drag-and-drop canvas      B: pick a layout + fill a form      C: block list + inspector
// All three write the same stored document (right panel) and preview through the one renderer.
import {
  Anchor,
  Button,
  Code,
  ColorInput,
  Container,
  Drawer,
  Group,
  NumberInput,
  ScrollArea,
  SegmentedControl,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import validator from '@rjsf/validator-ajv8'
import { useHistoryTravel } from 'ahooks'
import { useState } from 'react'
import { useSearchParams } from 'react-router'

import type { Device, FormDoc } from './doc.ts'
import { SITE_PALETTE, loadDocs, resetDocs, saveDoc } from './doc.ts'
import { resetFormState } from './runtime.ts'
import { DOC_SCHEMA } from './schema.ts'
import { Switcher } from './Switcher.tsx'
import { VariantOutline } from './VariantOutline.tsx'
import { VariantPuck } from './VariantPuck.tsx'
import { VariantTemplate } from './VariantTemplate.tsx'

const VARIANTS = [
  { key: 'A', name: 'Puck drag-and-drop canvas' },
  { key: 'B', name: 'Fixed layout + form' },
  { key: 'C', name: 'Block list + inspector (RJSF, dnd-kit)' },
]

export function Builder() {
  const [params] = useSearchParams()
  const variant = params.get('variant') ?? 'A'
  // Undo/redo for every variant from ahooks (Puck keeps its own history as well).
  const history = useHistoryTravel<FormDoc[]>(loadDocs())
  const docs = history.value ?? []
  const setDocs = (fn: (all: FormDoc[]) => FormDoc[]) => history.setValue(fn(docs))
  const [docId, setDocId] = useState(docs[0]?.id ?? '')
  const [device, setDevice] = useState<Device>('desktop')
  const [themeOpen, setThemeOpen] = useState(false)
  const [saved, setSaved] = useState('')
  const doc = docs.find((d) => d.id === docId) ?? docs[0]
  const change = (next: FormDoc) => setDocs((all) => all.map((d) => (d.id === next.id ? next : d)))
  const theme = (patch: Partial<FormDoc['theme']>) => change({ ...doc, theme: { ...doc.theme, ...patch } })
  // The same check the server would run at Publish, with the schemas the inspector draws from.
  const errors = validator.validateFormData(doc, DOC_SCHEMA).errors.map((e) => `${e.property ?? ''} ${e.message ?? ''}`)

  return (
    <Container fluid p="md" pb={80}>
      <Group justify="space-between" mb="sm">
        <Group>
          <Title order={3}>On-site form builder</Title>
          <Text c="red" fw={700} size="sm">PROTOTYPE</Text>
          <Select
            size="xs"
            value={doc.id}
            onChange={(v) => v && setDocId(v)}
            data={docs.map((d) => ({ value: d.id, label: `${d.name} (${d.format})` }))}
          />
          <SegmentedControl size="xs" value={device} onChange={(v) => setDevice(v)} data={['desktop', 'mobile']} />
          <Button size="xs" variant="light" onClick={() => setThemeOpen(true)}>Theme</Button>
          <Button size="xs" variant="default" disabled={!history.backLength} onClick={() => history.back()}>Undo</Button>
          <Button size="xs" variant="default" disabled={!history.forwardLength} onClick={() => history.forward()}>Redo</Button>
          {errors.length ? (
            <Text size="xs" c="red" title={errors.join('\n')}>{errors.length} schema errors</Text>
          ) : (
            <Text size="xs" c="green">valid against the document schema</Text>
          )}
        </Group>
        <Group>
          <Button
            size="xs"
            onClick={() => {
              saveDoc(doc)
              resetFormState()
              setSaved(new Date().toLocaleTimeString())
            }}
          >
            Publish to the shop
          </Button>
          {saved && <Text size="xs" c="green">published {saved}</Text>}
          <Anchor size="sm" href="/prototype/article" target="_blank">Open the shop article ↗</Anchor>
          <Button size="xs" variant="subtle" color="red" onClick={() => { resetDocs(); resetFormState(); location.reload() }}>Reset</Button>
        </Group>
      </Group>

      <Group align="flex-start" wrap="nowrap" gap="md">
        <div style={{ flex: 1, minWidth: 0 }}>
          {variant === 'A' && <VariantPuck doc={doc} onChange={change} device={device} />}
          {variant === 'B' && <VariantTemplate doc={doc} onChange={change} device={device} />}
          {variant === 'C' && <VariantOutline doc={doc} onChange={change} device={device} />}
        </div>
        <Stack gap={4} w={340}>
          <Text size="xs" c="dimmed">Stored document (live)</Text>
          <ScrollArea h={640}>
            <Code block style={{ fontSize: 11 }}>{JSON.stringify(doc, null, 2)}</Code>
          </ScrollArea>
          <Text size="xs" c="dimmed">{new Blob([JSON.stringify(doc)]).size} bytes</Text>
        </Stack>
      </Group>

      <Drawer opened={themeOpen} onClose={() => setThemeOpen(false)} title="Theme (copied into the form, not referenced)" position="right">
        <Stack>
          <Button variant="light" onClick={() => theme({ ...SITE_PALETTE })}>Use the site's palette (detected)</Button>
          <ColorInput label="Primary" value={doc.theme.primary} onChange={(v) => theme({ primary: v })} />
          <ColorInput label="Background" value={doc.theme.background} onChange={(v) => theme({ background: v })} />
          <ColorInput label="Text" value={doc.theme.text} onChange={(v) => theme({ text: v })} />
          <TextInput label="Font stack" value={doc.theme.font} onChange={(e) => theme({ font: e.currentTarget.value })} />
          <NumberInput label="Corner radius" value={doc.theme.radius} onChange={(v) => theme({ radius: Number(v) })} />
        </Stack>
      </Drawer>

      <Switcher variants={VARIANTS} />
    </Container>
  )
}
