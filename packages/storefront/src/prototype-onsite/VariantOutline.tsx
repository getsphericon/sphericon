// PROTOTYPE (throwaway, ticket #958). Variant C: an ordered block list per step with an inspector,
// built from ready-made parts: the inspector is RJSF (@rjsf/mantine, already in the app) drawn from
// each block's JSON Schema (schema.ts), reordering is @dnd-kit/sortable, undo/redo is ahooks'
// useHistoryTravel in the builder shell. No free positioning: blocks stack; Columns is the one
// layout tool. Mobile: a second RJSF form holds only what differs on a phone.
import { DndContext, PointerSensor, closestCenter, useSensor, useSensors } from '@dnd-kit/core'
import type { DragEndEvent } from '@dnd-kit/core'
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Badge, Button, Group, Menu, NavLink, Paper, ScrollArea, SimpleGrid, Stack, Tabs, Text } from '@mantine/core'
import Form from '@rjsf/mantine'
import validator from '@rjsf/validator-ajv8'
import { useState } from 'react'

import type { Block, BlockType, Device, FormDoc, Step } from './doc.ts'
import { uid } from './doc.ts'
import { Preview } from './Preview.tsx'
import { BLOCK_SCHEMAS, BLOCK_UI, MOBILE_PROPS, mobileSchema } from './schema.ts'

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

type StepKey = number | 'success'

function stepOf(doc: FormDoc, key: StepKey): Step | undefined {
  return key === 'success' ? doc.success : doc.steps[key]
}

function withStep(doc: FormDoc, key: StepKey, fn: (blocks: Block[]) => Block[]): FormDoc {
  if (key === 'success') {
    return { ...doc, success: { ...(doc.success ?? { id: 'success', blocks: [] }), blocks: fn(doc.success?.blocks ?? []) } }
  }
  return { ...doc, steps: doc.steps.map((s, i) => (i === key ? { ...s, blocks: fn(s.blocks) } : s)) }
}

function mapBlocks(blocks: Block[], id: string, fn: (b: Block) => Block | null): Block[] {
  return blocks.flatMap((b) => {
    if (b.id === id) {
      const r = fn(b)
      return r ? [r] : []
    }
    if (b.type === 'columns') {
      return [{ ...b, left: mapBlocks(b.left ?? [], id, fn), right: mapBlocks(b.right ?? [], id, fn) }]
    }
    return [b]
  })
}

function findBlock(blocks: Block[], id: string | null): Block | undefined {
  for (const b of blocks) {
    if (b.id === id) {
      return b
    }
    const inner = findBlock([...(b.left ?? []), ...(b.right ?? [])], id)
    if (inner) {
      return inner
    }
  }
  return undefined
}

const label = (b: Block) => `${b.type}${b.props.text ? `: ${String(b.props.text).slice(0, 20)}` : b.props.label ? `: ${String(b.props.label)}` : ''}`

function Row({ b, selected, onSelect, depth = 0 }: { b: Block; selected: string | null; onSelect: (id: string) => void; depth?: number }) {
  const sortable = useSortable({ id: b.id, disabled: depth > 0 })
  return (
    <div ref={sortable.setNodeRef} style={{ transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }}>
      <Group gap={4} wrap="nowrap" pl={depth * 14}>
        {depth === 0 && (
          <Text c="dimmed" style={{ cursor: 'grab' }} {...sortable.attributes} {...sortable.listeners}>
            ⋮⋮
          </Text>
        )}
        <NavLink
          label={label(b)}
          active={b.id === selected}
          onClick={() => onSelect(b.id)}
          rightSection={b.mobile?.hidden ? <Badge size="xs" color="gray">hidden on phone</Badge> : b.mobile?.props ? <Badge size="xs">phone override</Badge> : null}
        />
      </Group>
      {b.type === 'columns' &&
        (['left', 'right'] as const).map((side) => (
          <div key={side}>
            <Text size="xs" c="dimmed" pl={(depth + 1) * 14}>
              {side}
            </Text>
            {(b[side] ?? []).map((c) => (
              <Row key={c.id} b={c} selected={selected} onSelect={onSelect} depth={depth + 1} />
            ))}
          </div>
        ))}
    </div>
  )
}

// Empty strings and empty objects mean "linked": drop them.
function clean(mobile: { hidden?: boolean; props?: Record<string, unknown> }): Block['mobile'] {
  const props = Object.fromEntries(Object.entries(mobile.props ?? {}).filter(([, v]) => v !== undefined && v !== ''))
  const out = { ...(mobile.hidden ? { hidden: true } : {}), ...(Object.keys(props).length ? { props } : {}) }
  return Object.keys(out).length ? out : undefined
}

export function VariantOutline({ doc, onChange, device }: { doc: FormDoc; onChange: (d: FormDoc) => void; device: Device }) {
  const [stepKey, setStepKey] = useState<StepKey>(0)
  const [selected, setSelected] = useState<string | null>(null)
  const step = stepOf(doc, stepKey)
  const block = findBlock(step?.blocks ?? [], selected)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))

  const add = (type: BlockType) => {
    const b: Block = { id: uid('blk'), type, props: { ...DEFAULTS[type] }, ...(type === 'columns' ? { left: [], right: [] } : {}) }
    onChange(withStep(doc, stepKey, (bs) => [...bs, b]))
    setSelected(b.id)
  }
  const update = (fn: (b: Block) => Block | null) => {
    if (selected) {
      onChange(withStep(doc, stepKey, (bs) => mapBlocks(bs, selected, fn)))
    }
  }
  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (over && active.id !== over.id) {
      onChange(
        withStep(doc, stepKey, (bs) =>
          arrayMove(
            bs,
            bs.findIndex((b) => b.id === active.id),
            bs.findIndex((b) => b.id === over.id),
          ),
        ),
      )
    }
  }

  return (
    <SimpleGrid cols={3} spacing="md" style={{ gridTemplateColumns: '270px 1fr 320px' }}>
      <Paper withBorder p="xs">
        <Stack gap={4}>
          <Text fw={600} size="sm">
            Steps
          </Text>
          {doc.steps.map((s, i) => (
            <NavLink key={s.id} label={`Step ${i + 1}`} active={stepKey === i} onClick={() => setStepKey(i)} />
          ))}
          <NavLink label="Success" active={stepKey === 'success'} onClick={() => setStepKey('success')} />
          <Button size="xs" variant="light" onClick={() => onChange({ ...doc, steps: [...doc.steps, { id: uid('step'), blocks: [] }] })}>
            + Step
          </Button>
          <Text fw={600} size="sm" mt="sm">
            Blocks (drag ⋮⋮ to reorder)
          </Text>
          <ScrollArea h={340}>
            <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
              <SortableContext items={(step?.blocks ?? []).map((b) => b.id)} strategy={verticalListSortingStrategy}>
                {(step?.blocks ?? []).map((b) => (
                  <Row key={b.id} b={b} selected={selected} onSelect={setSelected} />
                ))}
              </SortableContext>
            </DndContext>
          </ScrollArea>
          <Menu>
            <Menu.Target>
              <Button size="xs">+ Block</Button>
            </Menu.Target>
            <Menu.Dropdown>
              {(Object.keys(DEFAULTS) as BlockType[]).map((t) => (
                <Menu.Item key={t} onClick={() => add(t)}>
                  {t}
                </Menu.Item>
              ))}
            </Menu.Dropdown>
          </Menu>
        </Stack>
      </Paper>

      <Preview doc={doc} device={device} />

      <Paper withBorder p="xs">
        {block ? (
          <Tabs key={block.id} defaultValue="all">
            <Tabs.List>
              <Tabs.Tab value="all">All devices</Tabs.Tab>
              <Tabs.Tab value="mobile" disabled={!MOBILE_PROPS[block.type] && block.type === 'columns'}>
                Phone
              </Tabs.Tab>
            </Tabs.List>
            <Tabs.Panel value="all" pt="xs">
              <Form
                schema={BLOCK_SCHEMAS[block.type]}
                uiSchema={{ ...BLOCK_UI[block.type], 'ui:submitButtonOptions': { norender: true } }}
                formData={block.props}
                validator={validator}
                liveValidate
                showErrorList={false}
                onChange={(e) => update((b) => ({ ...b, props: (e.formData ?? {}) as Record<string, unknown> }))}
              />
              <Button
                mt="xs"
                size="xs"
                color="red"
                variant="light"
                onClick={() => {
                  update(() => null)
                  setSelected(null)
                }}
              >
                Delete block
              </Button>
            </Tabs.Panel>
            <Tabs.Panel value="mobile" pt="xs">
              <Form
                schema={mobileSchema(block.type)}
                uiSchema={{ 'ui:submitButtonOptions': { norender: true }, props: Object.fromEntries((MOBILE_PROPS[block.type] ?? []).map((k) => [k, { 'ui:placeholder': `linked: ${String(block.props[k] ?? '')}` }])) }}
                formData={{ hidden: Boolean(block.mobile?.hidden), props: block.mobile?.props ?? {} }}
                validator={validator}
                onChange={(e) => update((b) => ({ ...b, mobile: clean(e.formData ?? {}) }))}
              />
            </Tabs.Panel>
          </Tabs>
        ) : (
          <Text size="sm" c="dimmed">
            Select a block
          </Text>
        )}
      </Paper>
    </SimpleGrid>
  )
}
