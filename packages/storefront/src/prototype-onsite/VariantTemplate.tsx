// PROTOTYPE (throwaway, ticket #958). Variant B: pick a fixed layout, fill a form. The author never
// sees blocks; the stored document is generated from the layout and the fields. Opening a document
// made elsewhere is lossy: only what the fields know survives (that is part of what to judge).
import { Alert, Badge, Card, Checkbox, Group, NumberInput, Paper, SimpleGrid, Stack, Switch, Tabs, Text, TextInput, Textarea } from '@mantine/core'
import { useEffect, useState } from 'react'

import type { Block, Device, FormDoc } from './doc.ts'
import { Preview } from './Preview.tsx'

type Layout = 'classic' | 'image-top' | 'image-left'
interface Fields {
  layout: Layout
  headline: string
  body: string
  image: string
  button: string
  email: boolean
  phone: boolean
  push: boolean
  disclosure: string
  success: string
  mobileHideImage: boolean
  mobileHeadlineSize: number | null // null = linked to desktop
  headlineSize: number
}

function infer(doc: FormDoc): Fields {
  const all = (blocks: Block[]): Block[] => blocks.flatMap((b) => [b, ...all([...(b.left ?? []), ...(b.right ?? [])])])
  const first = all(doc.steps[0]?.blocks ?? [])
  const rest = all(doc.steps.slice(1).flatMap((s) => s.blocks))
  const find = (bs: Block[], t: string) => bs.find((b) => b.type === t)
  const h = find(first, 'heading')
  const img = find(first, 'image')
  return {
    layout: find(first, 'columns') ? 'image-left' : img ? 'image-top' : 'classic',
    headline: String(h?.props.text ?? 'Headline'),
    headlineSize: Number(h?.props.size ?? 24),
    mobileHeadlineSize: h?.mobile?.props?.size !== undefined ? Number(h.mobile.props.size) : null,
    body: String(find(first, 'text')?.props.text ?? ''),
    image: String(img?.props.src ?? 'https://picsum.photos/seed/brightpath/480/200'),
    button: String(find(first, 'button')?.props.label ?? 'Subscribe'),
    email: Boolean(find(first, 'email')),
    phone: Boolean(find([...first, ...rest], 'phone-sms')),
    push: Boolean(find([...first, ...rest], 'push-button')),
    disclosure: String(find([...first, ...rest], 'phone-sms')?.props.disclosure ?? 'By texting START you agree to receive recurring marketing texts. Msg & data rates may apply. Reply STOP to cancel.'),
    success: String(find(doc.success?.blocks ?? [], 'heading')?.props.text ?? find(doc.success?.blocks ?? [], 'text')?.props.text ?? 'Thanks!'),
    mobileHideImage: Boolean(img?.mobile?.hidden),
  }
}

function build(doc: FormDoc, f: Fields): FormDoc {
  const heading: Block = {
    id: 't_h',
    type: 'heading',
    props: { text: f.headline, size: f.headlineSize },
    ...(f.mobileHeadlineSize !== null ? { mobile: { props: { size: f.mobileHeadlineSize } } } : {}),
  }
  const image: Block = { id: 't_img', type: 'image', props: { src: f.image, alt: '' }, ...(f.mobileHideImage ? { mobile: { hidden: true } } : {}) }
  const content: Block[] = [
    heading,
    { id: 't_body', type: 'text', props: { text: f.body } },
    ...(f.email ? [{ id: 't_email', type: 'email', props: { label: 'Email', placeholder: 'you@example.com', required: true } } as Block] : []),
    { id: 't_btn', type: 'button', props: { label: f.button, action: f.phone ? 'next' : 'submit' } },
    ...(f.push ? [{ id: 't_push', type: 'push-button', props: { label: 'Or get browser notifications' } } as Block] : []),
  ]
  const step1: Block[] =
    f.layout === 'classic' ? content : f.layout === 'image-top' ? [image, ...content] : [{ id: 't_cols', type: 'columns', props: {}, left: [image], right: content }]
  const steps = [{ id: 'step_1', blocks: step1 }]
  if (f.phone) {
    steps.push({
      id: 'step_2',
      blocks: [
        { id: 't_h2', type: 'heading', props: { text: 'Want texts too?', size: 22 } },
        { id: 't_sms', type: 'phone-sms', props: { keyword: 'START', number: '+15555550123', disclosure: f.disclosure } },
        { id: 't_skip', type: 'button', props: { label: 'No thanks', action: 'submit', variant: 'link' } },
      ],
    })
  }
  return { ...doc, steps, success: { id: 'success', blocks: [{ id: 't_s', type: 'heading', props: { text: f.success, size: 22 } }] } }
}

export function VariantTemplate({ doc, onChange, device }: { doc: FormDoc; onChange: (d: FormDoc) => void; device: Device }) {
  const [f, setF] = useState<Fields>(() => infer(doc))
  useEffect(() => {
    setF(infer(doc))
    // Re-infer only when another document is opened.
  }, [doc.id])
  const set = (patch: Partial<Fields>) => {
    const next = { ...f, ...patch }
    setF(next)
    onChange(build(doc, next))
  }

  return (
    <SimpleGrid cols={2} style={{ gridTemplateColumns: '380px 1fr' }}>
      <Paper withBorder p="sm">
        <Tabs defaultValue="content">
          <Tabs.List>
            <Tabs.Tab value="layout">Layout</Tabs.Tab>
            <Tabs.Tab value="content">Content</Tabs.Tab>
            <Tabs.Tab value="phone">Phone</Tabs.Tab>
          </Tabs.List>
          <Tabs.Panel value="layout" pt="sm">
            <SimpleGrid cols={3}>
              {(['classic', 'image-top', 'image-left'] as Layout[]).map((l) => (
                <Card key={l} withBorder p="xs" style={{ cursor: 'pointer', outline: f.layout === l ? '2px solid var(--mantine-color-blue-6)' : undefined }} onClick={() => set({ layout: l })}>
                  <Text size="xs" ta="center">{l}</Text>
                  <Stack gap={2} mt={4}>
                    {l === 'image-top' && <div style={{ height: 14, background: '#adb5bd' }} />}
                    <Group gap={2} wrap="nowrap" align="stretch">
                      {l === 'image-left' && <div style={{ width: 18, background: '#adb5bd' }} />}
                      <Stack gap={2} style={{ flex: 1 }}>
                        <div style={{ height: 5, background: '#495057' }} />
                        <div style={{ height: 3, background: '#ced4da' }} />
                        <div style={{ height: 6, background: '#228be6' }} />
                      </Stack>
                    </Group>
                  </Stack>
                </Card>
              ))}
            </SimpleGrid>
          </Tabs.Panel>
          <Tabs.Panel value="content" pt="sm">
            <Stack gap="xs">
              <TextInput label="Headline" value={f.headline} onChange={(e) => set({ headline: e.currentTarget.value })} />
              <NumberInput label="Headline size" value={f.headlineSize} onChange={(v) => set({ headlineSize: Number(v) })} />
              <Textarea label="Text" autosize value={f.body} onChange={(e) => set({ body: e.currentTarget.value })} />
              {f.layout !== 'classic' && <TextInput label="Image URL" value={f.image} onChange={(e) => set({ image: e.currentTarget.value })} />}
              <TextInput label="Button" value={f.button} onChange={(e) => set({ button: e.currentTarget.value })} />
              <Text size="sm" fw={600} mt="xs">Collect</Text>
              <Checkbox label="Email" checked={f.email} onChange={(e) => set({ email: e.currentTarget.checked })} />
              <Checkbox label="SMS (adds a step: text START from the phone)" checked={f.phone} onChange={(e) => set({ phone: e.currentTarget.checked })} />
              {f.phone && <Textarea label="SMS disclosure" autosize value={f.disclosure} onChange={(e) => set({ disclosure: e.currentTarget.value })} />}
              <Checkbox label="Web push button" checked={f.push} onChange={(e) => set({ push: e.currentTarget.checked })} />
              <TextInput label="Success message" value={f.success} onChange={(e) => set({ success: e.currentTarget.value })} />
            </Stack>
          </Tabs.Panel>
          <Tabs.Panel value="phone" pt="sm">
            <Stack gap="xs">
              <Alert variant="light" p="xs">Everything is linked to desktop unless changed here.</Alert>
              <Switch label="Hide the image on phones" checked={f.mobileHideImage} onChange={(e) => set({ mobileHideImage: e.currentTarget.checked })} />
              <Group justify="space-between">
                <Text size="sm">Headline size on phones</Text>
                {f.mobileHeadlineSize === null ? <Badge variant="light">linked</Badge> : <Badge>overridden</Badge>}
              </Group>
              <NumberInput value={f.mobileHeadlineSize ?? f.headlineSize} onChange={(v) => set({ mobileHeadlineSize: Number(v) })} />
              {f.mobileHeadlineSize !== null && (
                <Text size="xs" c="blue" style={{ cursor: 'pointer' }} onClick={() => set({ mobileHeadlineSize: null })}>
                  Relink to desktop
                </Text>
              )}
            </Stack>
          </Tabs.Panel>
        </Tabs>
      </Paper>
      <Preview doc={doc} device={device} />
    </SimpleGrid>
  )
}
