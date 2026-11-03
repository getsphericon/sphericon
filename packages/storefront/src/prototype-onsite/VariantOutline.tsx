// PROTOTYPE (throwaway, ticket #958). Variant C: an ordered block list per step with an
// inspector. No free positioning: blocks stack vertically; a Columns block is the one layout tool.
// Mobile: each property is linked to desktop until the author unlinks it on the Mobile tab.
import {
  ActionIcon,
  Badge,
  Button,
  Group,
  Menu,
  NavLink,
  NumberInput,
  Paper,
  ScrollArea,
  Select,
  SimpleGrid,
  Stack,
  Switch,
  Tabs,
  Text,
  TextInput,
  Textarea,
} from '@mantine/core'
import { useState } from 'react'

import type { Block, BlockType, Device, FormDoc, Step } from './doc.ts'
import { uid } from './doc.ts'
import { Preview } from './Preview.tsx'

const DEFAULTS: Record<BlockType, Record<string, unknown>> = {
  heading: { text: 'Heading', size: 22 },
  text: { text: 'Some text' },
  image: { src: 'https://picsum.photos/seed/new/480/200', alt: '' },
  email: { label: 'Email', placeholder: 'you@example.com', required: true },
  'phone-sms': { keyword: 'START', number: '+15555550123', disclosure: 'By texting START you agree to receive recurring marketing texts. Msg & data rates may apply. Reply STOP to cancel.' },
  'push-button': { label: 'Get browser notifications' },
  button: { label: 'Submit', action: 'submit' },
  columns: {},
}

// The fields an inspector shows per type; "mobile" marks the ones that may differ on a phone.
const FIELDS: Record<BlockType, { key: string; kind: 'text' | 'long' | 'number' | 'select' | 'bool'; options?: string[]; mobile?: boolean }[]> = {
  heading: [{ key: 'text', kind: 'text', mobile: true }, { key: 'size', kind: 'number', mobile: true }],
  text: [{ key: 'text', kind: 'long', mobile: true }],
  image: [{ key: 'src', kind: 'text', mobile: true }, { key: 'alt', kind: 'text' }],
  email: [{ key: 'label', kind: 'text' }, { key: 'placeholder', kind: 'text' }, { key: 'required', kind: 'bool' }],
  'phone-sms': [{ key: 'keyword', kind: 'text' }, { key: 'number', kind: 'text' }, { key: 'disclosure', kind: 'long' }],
  'push-button': [{ key: 'label', kind: 'text' }],
  button: [{ key: 'label', kind: 'text', mobile: true }, { key: 'action', kind: 'select', options: ['next', 'submit', 'close', 'url'] }, { key: 'url', kind: 'text' }, { key: 'variant', kind: 'select', options: ['solid', 'link'] }],
  columns: [],
}

type StepKey = number | 'success'

function stepOf(doc: FormDoc, key: StepKey): Step | undefined {
  return key === 'success' ? doc.success : doc.steps[key]
}

function withStep(doc: FormDoc, key: StepKey, fn: (blocks: Block[]) => Block[]): FormDoc {
  if (key === 'success') {return { ...doc, success: { ...(doc.success ?? { id: 'success', blocks: [] }), blocks: fn(doc.success?.blocks ?? []) } }}
  return { ...doc, steps: doc.steps.map((s, i) => (i === key ? { ...s, blocks: fn(s.blocks) } : s)) }
}

// Flatten columns children so they are selectable too (one level deep is enough here).
function mapBlocks(blocks: Block[], id: string, fn: (b: Block) => Block | null): Block[] {
  return blocks.flatMap((b) => {
    if (b.id === id) {
      const r = fn(b)
      return r ? [r] : []
    }
    if (b.type === 'columns') {return [{ ...b, left: mapBlocks(b.left ?? [], id, fn), right: mapBlocks(b.right ?? [], id, fn) }]}
    return [b]
  })
}

function findBlock(blocks: Block[], id: string | null): Block | undefined {
  for (const b of blocks) {
    if (b.id === id) {return b}
    const inner = findBlock([...(b.left ?? []), ...(b.right ?? [])], id)
    if (inner) {return inner}
  }
  return undefined
}

export function VariantOutline({ doc, onChange, device }: { doc: FormDoc; onChange: (d: FormDoc) => void; device: Device }) {
  const [stepKey, setStepKey] = useState<StepKey>(0)
  const [selected, setSelected] = useState<string | null>(null)
  const step = stepOf(doc, stepKey)
  const block = findBlock(step?.blocks ?? [], selected)

  const add = (type: BlockType) => {
    const b: Block = { id: uid('blk'), type, props: { ...DEFAULTS[type] }, ...(type === 'columns' ? { left: [], right: [] } : {}) }
    onChange(withStep(doc, stepKey, (bs) => [...bs, b]))
    setSelected(b.id)
  }
  const move = (id: string, delta: number) =>
    onChange(
      withStep(doc, stepKey, (bs) => {
        const i = bs.findIndex((b) => b.id === id)
        const j = i + delta
        if (i < 0 || j < 0 || j >= bs.length) {return bs}
        const next = [...bs]
        ;[next[i], next[j]] = [next[j], next[i]]
        return next
      }),
    )
  const update = (fn: (b: Block) => Block | null) => selected && onChange(withStep(doc, stepKey, (bs) => mapBlocks(bs, selected, fn)))

  const row = (b: Block, depth = 0) => (
    <Stack key={b.id} gap={0}>
      <Group gap={4} wrap="nowrap" pl={depth * 14}>
        <NavLink
          label={`${b.type}${b.props.text ? `: ${String(b.props.text).slice(0, 18)}` : b.props.label ? `: ${String(b.props.label)}` : ''}`}
          active={b.id === selected}
          onClick={() => setSelected(b.id)}
          rightSection={b.mobile?.hidden ? <Badge size="xs" color="gray">hidden on phone</Badge> : b.mobile?.props ? <Badge size="xs">phone override</Badge> : null}
        />
        {depth === 0 && (
          <>
            <ActionIcon size="sm" variant="subtle" onClick={() => move(b.id, -1)}>↑</ActionIcon>
            <ActionIcon size="sm" variant="subtle" onClick={() => move(b.id, 1)}>↓</ActionIcon>
          </>
        )}
      </Group>
      {b.type === 'columns' && (
        <>
          <Text size="xs" c="dimmed" pl={(depth + 1) * 14}>left</Text>
          {(b.left ?? []).map((c) => row(c, depth + 1))}
          <Text size="xs" c="dimmed" pl={(depth + 1) * 14}>right</Text>
          {(b.right ?? []).map((c) => row(c, depth + 1))}
        </>
      )}
    </Stack>
  )

  return (
    <SimpleGrid cols={3} spacing="md" style={{ gridTemplateColumns: '260px 1fr 300px' }}>
      <Paper withBorder p="xs">
        <Stack gap={4}>
          <Text fw={600} size="sm">Steps</Text>
          {doc.steps.map((_, i) => (
            <NavLink key={i} label={`Step ${i + 1}`} active={stepKey === i} onClick={() => setStepKey(i)} />
          ))}
          <NavLink label="Success" active={stepKey === 'success'} onClick={() => setStepKey('success')} />
          <Button size="xs" variant="light" onClick={() => onChange({ ...doc, steps: [...doc.steps, { id: uid('step'), blocks: [] }] })}>
            + Step
          </Button>
          <Text fw={600} size="sm" mt="sm">Blocks</Text>
          <ScrollArea h={360}>{(step?.blocks ?? []).map((b) => row(b))}</ScrollArea>
          <Menu>
            <Menu.Target>
              <Button size="xs">+ Block</Button>
            </Menu.Target>
            <Menu.Dropdown>
              {(Object.keys(DEFAULTS) as BlockType[]).map((t) => (
                <Menu.Item key={t} onClick={() => add(t)}>{t}</Menu.Item>
              ))}
            </Menu.Dropdown>
          </Menu>
        </Stack>
      </Paper>

      <Preview doc={doc} device={device} />

      <Paper withBorder p="xs">
        {!block ? (
          <Text size="sm" c="dimmed">Select a block</Text>
        ) : (
          <Tabs defaultValue="all">
            <Tabs.List>
              <Tabs.Tab value="all">All devices</Tabs.Tab>
              <Tabs.Tab value="mobile">Phone</Tabs.Tab>
            </Tabs.List>
            <Tabs.Panel value="all" pt="xs">
              <Stack gap="xs">
                {FIELDS[block.type].map((f) => (
                  <Field key={f.key} f={f} value={block.props[f.key]} onChange={(v) => update((b) => ({ ...b, props: { ...b.props, [f.key]: v } }))} />
                ))}
                <Button size="xs" color="red" variant="light" onClick={() => { update(() => null); setSelected(null) }}>
                  Delete block
                </Button>
              </Stack>
            </Tabs.Panel>
            <Tabs.Panel value="mobile" pt="xs">
              <Stack gap="xs">
                <Switch
                  label="Hide on phones"
                  checked={Boolean(block.mobile?.hidden)}
                  onChange={(e) => {
                    const hidden = e.currentTarget.checked
                    update((b) => ({ ...b, mobile: { ...b.mobile, hidden } }))
                  }}
                />
                {FIELDS[block.type].filter((f) => f.mobile).map((f) => {
                  const linked = !(block.mobile?.props && f.key in block.mobile.props)
                  return (
                    <Stack key={f.key} gap={2}>
                      <Group justify="space-between">
                        <Text size="xs">{f.key}</Text>
                        <Button
                          size="compact-xs"
                          variant={linked ? 'light' : 'filled'}
                          onClick={() =>
                            update((b) => {
                              const props = { ...b.mobile?.props }
                              if (linked) {props[f.key] = b.props[f.key]}
                              else {delete props[f.key]}
                              return { ...b, mobile: { ...b.mobile, props: Object.keys(props).length > 0 ? props : undefined } }
                            })
                          }
                        >
                          {linked ? 'Linked to desktop · unlink' : 'Unlinked · relink'}
                        </Button>
                      </Group>
                      {!linked && (
                        <Field f={f} value={block.mobile?.props?.[f.key]} onChange={(v) => update((b) => ({ ...b, mobile: { ...b.mobile, props: { ...b.mobile?.props, [f.key]: v } } }))} />
                      )}
                    </Stack>
                  )
                })}
              </Stack>
            </Tabs.Panel>
          </Tabs>
        )}
      </Paper>
    </SimpleGrid>
  )
}

function Field({ f, value, onChange }: { f: (typeof FIELDS)[BlockType][number]; value: unknown; onChange: (v: unknown) => void }) {
  if (f.kind === 'long') {return <Textarea label={f.key} autosize value={String(value ?? '')} onChange={(e) => onChange(e.currentTarget.value)} />}
  if (f.kind === 'number') {return <NumberInput label={f.key} value={Number(value ?? 0)} onChange={(v) => onChange(Number(v))} />}
  if (f.kind === 'select') {return <Select label={f.key} data={f.options ?? []} value={String(value ?? '')} onChange={(v) => onChange(v)} />}
  if (f.kind === 'bool') {return <Switch label={f.key} checked={Boolean(value)} onChange={(e) => onChange(e.currentTarget.checked)} />}
  return <TextInput label={f.key} value={String(value ?? '')} onChange={(e) => onChange(e.currentTarget.value)} />
}
