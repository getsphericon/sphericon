// PROTOTYPE (throwaway, ticket #958). Variant A: Puck's drag-and-drop canvas (MIT, React 19).
// Puck stores {content: [{type, props}], root}; Columns use slot fields. It edits one step at a
// time, mapped onto the shared document. Puck draws its canvas with OUR React render functions,
// which are not the shipped renderer: the true render is shown beside it, so the gap is visible.
import type { Config, Data } from '@puckeditor/core'
import { Puck } from '@puckeditor/core'
import '@puckeditor/core/puck.css'
import { Group, Paper, SegmentedControl, Stack, Text } from '@mantine/core'
import type { ReactNode } from 'react'
import { useMemo, useState } from 'react'

import type { Block, BlockType, Device, FormDoc } from './doc.ts'
import { Preview } from './Preview.tsx'

const TO_PUCK: Record<BlockType, string> = {
  heading: 'Heading',
  text: 'Text',
  image: 'Image',
  email: 'EmailField',
  'phone-sms': 'SmsOptIn',
  'push-button': 'PushButton',
  button: 'Button',
  columns: 'Columns',
}
const FROM_PUCK = Object.fromEntries(Object.entries(TO_PUCK).map(([k, v]) => [v, k])) as Record<string, BlockType>

interface PuckItem { type: string; props: Record<string, unknown> & { id: string } }

// Puck has preview viewports but no per-viewport props: phone overrides become ordinary fields.
function toPuck(blocks: Block[]): PuckItem[] {
  return blocks.map((b) => ({
    type: TO_PUCK[b.type],
    props: {
      id: b.id,
      ...b.props,
      phoneHidden: b.mobile?.hidden ? 'yes' : 'no',
      ...(b.mobile?.props?.size !== undefined ? { phoneSize: b.mobile.props.size } : {}),
      ...(b.type === 'columns' ? { left: toPuck(b.left ?? []), right: toPuck(b.right ?? []) } : {}),
    },
  }))
}

function fromPuck(items: PuckItem[]): Block[] {
  return items.map(({ type, props }) => {
    const { id, phoneHidden, phoneSize, left, right, ...rest } = props
    const t = FROM_PUCK[type]
    const mobileProps = phoneSize !== undefined && phoneSize !== null && phoneSize !== '' ? { size: Number(phoneSize) } : undefined
    const hidden = phoneHidden === 'yes'
    return {
      id,
      type: t,
      props: rest,
      ...(hidden || mobileProps ? { mobile: { ...(hidden ? { hidden } : {}), ...(mobileProps ? { props: mobileProps } : {}) } } : {}),
      ...(t === 'columns' ? { left: fromPuck((left ?? []) as PuckItem[]), right: fromPuck((right ?? []) as PuckItem[]) } : {}),
    }
  })
}

const phoneFields = {
  phoneHidden: { type: 'radio' as const, label: 'Phone: hide', options: [{ label: 'no', value: 'no' }, { label: 'yes', value: 'yes' }] },
}
const box = (children: ReactNode) => <div style={{ padding: '4px 0', fontFamily: 'system-ui' }}>{children}</div>

const config: Config = {
  components: {
    Heading: {
      fields: { text: { type: 'text' }, size: { type: 'number' }, phoneSize: { type: 'number', label: 'Phone: size (empty = linked)' }, ...phoneFields },
      defaultProps: { text: 'Heading', size: 22, phoneHidden: 'no' },
      render: ({ text, size }) => box(<h2 style={{ margin: 0, fontSize: Number(size) }}>{String(text)}</h2>),
    },
    Text: {
      fields: { text: { type: 'textarea' }, ...phoneFields },
      defaultProps: { text: 'Some text', phoneHidden: 'no' },
      render: ({ text }) => box(<p style={{ margin: 0 }}>{String(text)}</p>),
    },
    Image: {
      fields: { src: { type: 'text' }, alt: { type: 'text' }, ...phoneFields },
      defaultProps: { src: 'https://picsum.photos/seed/puck/480/200', alt: '', phoneHidden: 'no' },
      render: ({ src }) => box(<img src={String(src)} alt="" style={{ width: '100%' }} />),
    },
    EmailField: {
      fields: { label: { type: 'text' }, placeholder: { type: 'text' }, required: { type: 'radio', options: [{ label: 'yes', value: true }, { label: 'no', value: false }] } },
      defaultProps: { label: 'Email', placeholder: 'you@example.com', required: true },
      render: ({ label, placeholder }) => box(<label>{String(label)}<br /><input placeholder={String(placeholder)} readOnly /></label>),
    },
    SmsOptIn: {
      fields: { keyword: { type: 'text' }, number: { type: 'text' }, disclosure: { type: 'textarea' } },
      defaultProps: { keyword: 'START', number: '+15555550123', disclosure: 'By texting START you agree to receive recurring marketing texts. Reply STOP to cancel.' },
      render: ({ keyword, disclosure }) => box(<><button type="button">Text {String(keyword)} to join</button><small style={{ display: 'block' }}>{String(disclosure)}</small></>),
    },
    PushButton: {
      fields: { label: { type: 'text' } },
      defaultProps: { label: 'Get browser notifications' },
      render: ({ label }) => box(<u>{String(label)}</u>),
    },
    Button: {
      fields: {
        label: { type: 'text' },
        action: { type: 'select', options: ['next', 'submit', 'close', 'url'].map((v) => ({ label: v, value: v })) },
        url: { type: 'text' },
        variant: { type: 'select', options: ['solid', 'link'].map((v) => ({ label: v, value: v })) },
      },
      defaultProps: { label: 'Submit', action: 'submit', variant: 'solid' },
      render: ({ label }) => box(<button type="button">{String(label)}</button>),
    },
    Columns: {
      fields: { left: { type: 'slot' }, right: { type: 'slot' } },
      render: ({ left: Left, right: Right }) => (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
          <Left />
          <Right />
        </div>
      ),
    },
  },
}

export function VariantPuck({ doc, onChange, device }: { doc: FormDoc; onChange: (d: FormDoc) => void; device: Device }) {
  const [stepKey, setStepKey] = useState('0')
  const steps = [...doc.steps.map((_, i) => String(i)), ...(doc.success ? ['success'] : [])]
  const key = steps.includes(stepKey) ? stepKey : '0'
  const blocks = key === 'success' ? (doc.success?.blocks ?? []) : doc.steps[Number(key)].blocks
  // Puck is uncontrolled after mount: remount it when the document or step changes.
  const data: Data = useMemo(() => ({ root: { props: {} }, content: toPuck(blocks) as Data['content'] }), [doc.id, key])

  const save = (d: Data) => {
    const next = fromPuck(d.content)
    if (key === 'success') {onChange({ ...doc, success: { id: 'success', blocks: next } })}
    else {onChange({ ...doc, steps: doc.steps.map((s, i) => (i === Number(key) ? { ...s, blocks: next } : s)) })}
  }

  return (
    <Stack gap="xs">
      <Group>
        <Text size="sm">Editing:</Text>
        <SegmentedControl size="xs" value={key} onChange={setStepKey} data={steps.map((s) => ({ value: s, label: s === 'success' ? 'Success' : `Step ${Number(s) + 1}` }))} />
        <Text size="xs" c="dimmed">Drag blocks from the left; Columns has two drop zones.</Text>
      </Group>
      <Paper withBorder style={{ height: 600, overflow: 'hidden' }}>
        <Puck
          key={`${doc.id}-${key}`}
          config={config}
          data={data}
          onChange={save}
          viewports={[
            { width: 480, height: 'auto', label: 'Desktop popup' },
            { width: 375, height: 'auto', label: 'Phone' },
          ]}
        />
      </Paper>
      <Preview doc={doc} device={device} title="Shipped renderer (what the site gets; Puck's canvas above is a second renderer)" />
    </Stack>
  )
}
