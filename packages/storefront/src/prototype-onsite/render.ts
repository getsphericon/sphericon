// PROTOTYPE (throwaway, branch prototype/onsite-form-builder, ticket #958). Not production code.
//
// The one renderer, now on Preact (~4.8 kB) instead of hand-written DOM, so blocks are components
// written once: the builder previews, Puck's canvas (renderBlock) and the storefront all draw
// through it. Real QR codes come from `uqr`. React alone would be ~99 kB gzip.
//
// Isolation (the assumption under test): an OPEN shadow root on a host element in the page's own
// document, `:host { all: initial }`, styles through adoptedStyleSheets (CSSOM, not blocked by
// style-src in any browser per MDN), and a <dialog> opened with showModal() for the popup.
// Why not an iframe: a cross-origin frame cannot ask for web push permission, and a modal inside a
// frame leaves the parent page reachable (research/on-site-runtime, sections 2.4 and 2.5).
//
// Bot protection: ALTCHA's widget is ~58 kB gzip and its solver library ~23 kB, so neither is in
// this bundle; the solver would load only on submit (finding for ADR 0088).

import { h, render } from 'preact'
import type { VNode } from 'preact'
import { useEffect, useRef, useState } from 'preact/hooks'
import { encode } from 'uqr'

import type { Block, Device, FormDoc, Theme } from './doc.ts'
import { propsFor } from './doc.ts'

export type FormEvent = 'shown' | 'closed' | 'submitted' | 'clicked' | 'step'
export interface RenderOptions {
  device: Device
  mode: 'live' | 'preview'
  container?: HTMLElement
  start?: number | 'success' | 'teaser'
  onEvent?: (event: FormEvent, data?: Record<string, unknown>) => void
}

const CSS = `
:host { all: initial; display: block; }
:host([data-format="popup"]), :host([data-format="slide-in"]), :host([data-format="bar"]) { display: contents; }
* { box-sizing: border-box; }
.theme { font-family: var(--font); color: var(--text); font-size: 16px; line-height: 1.45; }
.card { background: var(--bg); border-radius: var(--radius); padding: 24px; display: flex; flex-direction: column; gap: 12px; position: relative; }
.popup .card { width: min(440px, 92vw); box-shadow: 0 12px 40px rgba(0,0,0,.25); }
.mobile .card { padding: 18px; }
dialog { border: 0; padding: 0; background: transparent; max-width: none; }
dialog::backdrop { background: rgba(0,0,0,.45); }
.slide { position: fixed; right: 16px; bottom: 16px; z-index: 2147483000; width: min(360px, 92vw); }
.bar { position: fixed; left: 0; right: 0; top: 0; z-index: 2147483000; }
.bar .card { border-radius: 0; flex-direction: row; align-items: center; justify-content: center; padding: 10px 16px; }
.preview-overlay { background: rgba(0,0,0,.35); padding: 32px 12px; display: flex; justify-content: center; border-radius: 6px; }
.x { position: absolute; top: 8px; right: 10px; border: 0; background: transparent; font: inherit; font-size: 22px; color: var(--text); cursor: pointer; line-height: 1; }
h2 { margin: 0; font-weight: 700; line-height: 1.2; }
p { margin: 0; }
img { width: 100%; display: block; border-radius: calc(var(--radius) / 2); }
label { display: flex; flex-direction: column; gap: 4px; font-size: 14px; }
input { font: inherit; padding: 10px 12px; border: 1px solid #ced4da; border-radius: calc(var(--radius) / 2); color: #1a1b1e; background: #fff; }
.btn { font: inherit; font-weight: 600; padding: 10px 16px; border: 0; border-radius: var(--radius); background: var(--primary); color: #fff; cursor: pointer; text-align: center; text-decoration: none; display: block; }
.btn.link { background: transparent; color: var(--text); text-decoration: underline; font-weight: 400; padding: 4px; }
.cols { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
.mobile .cols { grid-template-columns: 1fr; }
.col { display: flex; flex-direction: column; gap: 10px; }
.small { font-size: 12px; opacity: .75; }
.qr { width: 140px; height: 140px; align-self: center; }
.teaser { position: fixed; left: 16px; bottom: 16px; z-index: 2147483000; font-family: var(--font); background: var(--primary); color: #fff;
  border: 0; border-radius: 999px; padding: 10px 18px; font-weight: 700; cursor: pointer; box-shadow: 0 6px 20px rgba(0,0,0,.25); }
.preview-teaser .teaser { position: static; }
.note { font-size: 11px; color: #c92a2a; }
`

// A constructed stylesheet belongs to the document that made it, so a host inside another document
// (Puck draws its canvas in an iframe) needs a sheet built from that document's own constructor.
const sheets = new WeakMap<Document, CSSStyleSheet>()
function styles(doc: Document): CSSStyleSheet {
  let sheet = sheets.get(doc)
  if (!sheet) {
    const Ctor = (doc.defaultView as (Window & typeof globalThis) | null)?.CSSStyleSheet ?? CSSStyleSheet
    sheet = new Ctor()
    sheet.replaceSync(CSS)
    sheets.set(doc, sheet)
  }
  return sheet
}

// Theme as custom properties, set through CSSOM (no style attribute, no style-src needed).
function themeRef(theme: Theme) {
  return (node: HTMLElement | null) => {
    if (!node) {
      return
    }
    node.style.setProperty('--primary', theme.primary)
    node.style.setProperty('--bg', theme.background)
    node.style.setProperty('--text', theme.text)
    node.style.setProperty('--font', theme.font)
    node.style.setProperty('--radius', `${theme.radius}px`)
  }
}

function shadowHost(format: string, id: string, owner: Document): { host: HTMLElement; root: ShadowRoot } {
  const host = owner.createElement('div')
  host.dataset.format = format
  host.setAttribute('data-sphericon-form', id)
  const root = host.attachShadow({ mode: 'open' })
  root.adoptedStyleSheets = [styles(owner)]
  return { host, root }
}

// A real QR code (uqr, MIT) for `SMSTO:<number>:START <token>` (ADR 0088), built as DOM nodes.
function Qr({ text }: { text: string }) {
  const { data, size } = encode(text, { border: 1 })
  const rects: VNode[] = []
  data.forEach((row, y) => row.forEach((on, x) => on && rects.push(h('rect', { x, y, width: 1, height: 1 }))))
  return h('svg', { class: 'qr', viewBox: `0 0 ${size} ${size}`, 'shape-rendering': 'crispEdges' }, rects)
}

interface Ctx {
  device: Device
  mode: 'live' | 'preview'
  emit: (event: FormEvent, data?: Record<string, unknown>) => void
  act: (action: string) => void
}

function BlockView({ b, ctx }: { b: Block; ctx: Ctx }): VNode | null {
  if (ctx.device === 'mobile' && b.mobile?.hidden) {
    return null
  }
  const p = propsFor(b, ctx.device)
  const s = (k: string, d = '') => String(p[k] ?? d)
  switch (b.type) {
    case 'heading':
      return h('h2', { ref: (n: HTMLElement | null) => n?.style.setProperty('font-size', `${Number(p.size ?? 22)}px`) }, s('text'))
    case 'text':
      return h('p', null, s('text'))
    case 'image':
      return h('img', { src: s('src'), alt: s('alt') })
    case 'email':
      return h('label', null, s('label', 'Email'), h('input', { type: 'email', name: 'email', autocomplete: 'email', placeholder: s('placeholder'), required: Boolean(p.required) }))
    case 'phone-sms': {
      const body = `${s('keyword', 'START')} tok_demo123`
      return h(
        'div',
        { class: 'col' },
        ctx.device === 'mobile'
          ? h('a', { class: 'btn', href: `sms:${s('number')}?&body=${encodeURIComponent(body)}`, onClick: () => ctx.emit('clicked', { element: 'phone-sms' }) }, `Text ${s('keyword', 'START')} to join`)
          : [h(Qr, { text: `SMSTO:${s('number')}:${body}` }), h('p', { class: 'small' }, `Scan with your phone to text ${s('keyword', 'START')} to ${s('number')}`)],
        h('p', { class: 'small' }, s('disclosure')),
      )
    }
    case 'push-button':
      return h(PushButton, { label: s('label', 'Get notifications'), ctx })
    case 'button': {
      const action = s('action', 'submit')
      return h(
        'button',
        {
          type: 'button',
          class: `btn${p.variant === 'link' ? ' link' : ''}`,
          onClick: () => {
            ctx.emit('clicked', { element: 'button', action })
            ctx.act(action === 'url' ? `url:${s('url', '#')}` : action)
          },
        },
        s('label', 'Submit'),
      )
    }
    case 'columns':
      return h(
        'div',
        { class: 'cols' },
        [b.left ?? [], b.right ?? []].map((side) => h('div', { class: 'col' }, side.map((c) => h(BlockView, { key: c.id, b: c, ctx })))),
      )
    default:
      return null
  }
}

function PushButton({ label, ctx }: { label: string; ctx: Ctx }) {
  const permission = typeof Notification === 'undefined' ? 'unsupported' : Notification.permission
  const [result, setResult] = useState('')
  // ADR 0092: hidden once the browser has allowed or blocked; the preview shows it with a note.
  if (permission !== 'default' && ctx.mode === 'live') {
    return null
  }
  return h(
    'div',
    { class: 'col' },
    h(
      'button',
      {
        type: 'button',
        class: 'btn link',
        onClick: () => {
          ctx.emit('clicked', { element: 'push-button' })
          // A click inside the shadow root is a user gesture in the page's own document.
          void Notification.requestPermission().then((r) => setResult(r))
        },
      },
      result ? `Permission: ${result}` : label,
    ),
    permission !== 'default' && h('span', { class: 'note' }, `(preview: hidden live, permission is ${permission})`),
  )
}

function FormApp({ doc, opts, onRemove }: { doc: FormDoc; opts: RenderOptions; onRemove: () => void }) {
  const emit = opts.onEvent ?? (() => undefined)
  const preview = opts.mode === 'preview'
  const teaserFirst =
    preview ? opts.start === 'teaser' : doc.format === 'popup' && Boolean(doc.teaser) && (doc.teaser?.show !== 'after' || opts.device === 'mobile')
  const [step, setStep] = useState<number | 'success'>(typeof opts.start === 'number' || opts.start === 'success' ? opts.start : 0)
  const [open, setOpen] = useState(!teaserFirst)
  const [teaser, setTeaser] = useState(teaserFirst)
  const [values, setValues] = useState<Record<string, string>>({})
  const card = useRef<HTMLDivElement>(null)
  const dialog = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    if (open) {
      emit('shown', { device: opts.device })
      if (!preview && doc.format === 'popup') {
        dialog.current?.showModal()
      }
    }
  }, [open])

  const close = () => {
    emit('closed', { step })
    if (preview) {
      return
    }
    dialog.current?.close()
    setOpen(false)
    if (doc.teaser && doc.teaser.show !== 'before' && step !== 'success') {
      setTeaser(true)
    } else if (doc.format !== 'popup') {
      onRemove()
    }
  }

  const act = (action: string) => {
    if (action === 'close') {
      close()
      return
    }
    if (action.startsWith('url:')) {
      window.open(action.slice(4), '_blank')
      return
    }
    const next = { ...values }
    card.current?.querySelectorAll('input').forEach((i) => {
      if (i.name) {
        next[i.name] = i.value
      }
    })
    setValues(next)
    if (action === 'next' && typeof step === 'number' && step < doc.steps.length - 1) {
      setStep(step + 1)
      emit('step', { step: step + 1, values: next })
    } else {
      emit('submitted', { values: next })
      if (doc.success) {
        setStep('success')
      } else {
        close()
      }
    }
  }

  const ctx: Ctx = { device: opts.device, mode: opts.mode, emit, act }
  const current = step === 'success' ? doc.success : doc.steps[step]
  const cardNode = h(
    'div',
    { class: 'card', ref: card },
    doc.format !== 'embedded' && h('button', { type: 'button', class: 'x', 'aria-label': 'Close', onClick: close }, '×'),
    (current?.blocks ?? []).map((b) => h(BlockView, { key: b.id, b, ctx })),
  )
  const teaserNode =
    teaser && doc.teaser
      ? h(
          'button',
          {
            type: 'button',
            class: 'teaser',
            onClick: () => {
              setTeaser(false)
              setOpen(true)
            },
          },
          doc.teaser.text,
        )
      : null

  let body: VNode | null = null
  if (open && !(preview && opts.start === 'teaser')) {
    if (preview) {
      body = doc.format === 'popup' ? h('div', { class: 'preview-overlay' }, cardNode) : cardNode
    } else if (doc.format === 'popup') {
      body = h('dialog', { ref: dialog, 'aria-label': doc.name, onCancel: () => close() }, cardNode)
    } else {
      body = h('div', { class: doc.format === 'slide-in' ? 'slide' : doc.format === 'bar' ? 'bar' : '' }, cardNode)
    }
  }
  return h('div', { class: `theme ${doc.format} ${opts.device}${preview && opts.start === 'teaser' ? ' preview-teaser' : ''}`, ref: themeRef(doc.theme) }, body, teaserNode)
}

export function renderForm(doc: FormDoc, opts: RenderOptions): { host: HTMLElement; destroy: () => void } {
  const target = opts.container ?? document.body
  const { host, root } = shadowHost(doc.format, doc.id, target.ownerDocument)
  target.append(host)
  const destroy = () => {
    render(null, root)
    host.remove()
  }
  render(h(FormApp, { doc, opts, onRemove: destroy }), root)
  return { host, destroy }
}

// One block on its own, for Puck's canvas: the same component and styles the site gets.
export function renderBlock(block: Block, theme: Theme, device: Device, container: HTMLElement): () => void {
  const { host, root } = shadowHost('embedded', block.id, container.ownerDocument)
  container.append(host)
  const ctx: Ctx = { device, mode: 'preview', emit: () => undefined, act: () => undefined }
  render(h('div', { class: `theme ${device}`, ref: themeRef(theme) }, h(BlockView, { b: block, ctx })), root)
  return () => {
    render(null, root)
    host.remove()
  }
}
