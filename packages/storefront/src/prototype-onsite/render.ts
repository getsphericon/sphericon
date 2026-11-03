// PROTOTYPE (throwaway, branch prototype/onsite-form-builder, ticket #958). Not production code.
//
// The one renderer: plain DOM, no React, the kind the tracker could ship (React alone is ~99 kB
// gzip against a 17 kB tracker, research/on-site-builders). Every builder preview and the
// storefront page call this same function, so what the author sees is what ships.
//
// Isolation (the assumption under test): an OPEN shadow root on a host element in the page's own
// document, `:host { all: initial }`, styles through adoptedStyleSheets (CSSOM, not blocked by
// style-src in any browser per MDN), and a <dialog> opened with showModal() for the popup.
// Why not an iframe: a cross-origin frame cannot ask for web push permission, and a modal inside a
// frame leaves the parent page reachable (research/on-site-runtime, sections 2.4 and 2.5).

import type { Block, Device, FormDoc, Step } from './doc.ts'
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
.card { font-family: var(--font); color: var(--text); background: var(--bg); border-radius: var(--radius);
  padding: 24px; font-size: 16px; line-height: 1.45; display: flex; flex-direction: column; gap: 12px; }
.popup .card { width: min(440px, 92vw); box-shadow: 0 12px 40px rgba(0,0,0,.25); }
.mobile .card { padding: 18px; }
dialog { border: 0; padding: 0; background: transparent; max-width: none; }
dialog::backdrop { background: rgba(0,0,0,.45); }
.slide { position: fixed; right: 16px; bottom: 16px; z-index: 2147483000; width: min(360px, 92vw); }
.bar { position: fixed; left: 0; right: 0; top: 0; z-index: 2147483000; }
.bar .card { border-radius: 0; flex-direction: row; align-items: center; justify-content: center; padding: 10px 16px; }
.preview-overlay { background: rgba(0,0,0,.35); padding: 32px 12px; display: flex; justify-content: center; border-radius: 6px; }
.x { position: absolute; top: 8px; right: 10px; border: 0; background: transparent; font: inherit; font-size: 22px; color: var(--text); cursor: pointer; line-height: 1; }
.rel { position: relative; }
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
.qr { width: 132px; height: 132px; align-self: center; }
.teaser { position: fixed; left: 16px; bottom: 16px; z-index: 2147483000; font-family: var(--font); background: var(--primary); color: #fff;
  border: 0; border-radius: 999px; padding: 10px 18px; font-weight: 700; cursor: pointer; box-shadow: 0 6px 20px rgba(0,0,0,.25); }
.preview-teaser .teaser { position: static; }
.note { font-size: 11px; color: #c92a2a; }
`

let sheet: CSSStyleSheet | null = null
function styles(): CSSStyleSheet {
  if (!sheet) {
    sheet = new CSSStyleSheet()
    sheet.replaceSync(CSS)
  }
  return sheet
}

function el<K extends keyof HTMLElementTagNameMap>(tag: K, cls?: string, text?: string): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  if (cls) {node.className = cls}
  if (text !== undefined) {node.textContent = text}
  return node
}

// A fake QR code: the real one encodes `SMSTO:<number>:START <token>` (ADR 0088).
function qr(seed: string): SVGSVGElement {
  const ns = 'http://www.w3.org/2000/svg'
  const svg = document.createElementNS(ns, 'svg')
  svg.setAttribute('viewBox', '0 0 21 21')
  svg.setAttribute('class', 'qr')
  let h = 0
  for (const ch of seed) {h = (h * 31 + ch.charCodeAt(0)) >>> 0}
  for (let y = 0; y < 21; y++) {
    for (let x = 0; x < 21; x++) {
      const finder = (x < 7 && y < 7) || (x > 13 && y < 7) || (x < 7 && y > 13)
      h = (h * 1103515245 + 12345) >>> 0
      const on = finder ? x % 6 === 0 || y % 6 === 0 || (x % 7 > 1 && x % 7 < 5 && y % 7 > 1 && y % 7 < 5) : h % 3 === 0
      if (on) {
        const r = document.createElementNS(ns, 'rect')
        r.setAttribute('x', String(x))
        r.setAttribute('y', String(y))
        r.setAttribute('width', '1')
        r.setAttribute('height', '1')
        svg.append(r)
      }
    }
  }
  return svg
}

export function renderForm(doc: FormDoc, opts: RenderOptions): { host: HTMLElement; destroy: () => void } {
  const emit = opts.onEvent ?? (() => undefined)
  const host = el('div')
  host.dataset.format = doc.format
  host.setAttribute('data-sphericon-form', doc.id)
  const root = host.attachShadow({ mode: 'open' })
  root.adoptedStyleSheets = [styles()]

  const wrap = el('div', `${doc.format} ${opts.device}`)
  // Theme as custom properties through CSSOM: no style attribute, so no style-src needed.
  wrap.style.setProperty('--primary', doc.theme.primary)
  wrap.style.setProperty('--bg', doc.theme.background)
  wrap.style.setProperty('--text', doc.theme.text)
  wrap.style.setProperty('--font', doc.theme.font)
  wrap.style.setProperty('--radius', `${doc.theme.radius}px`)
  root.append(wrap)

  const values: Record<string, string> = {}
  let stepIndex: number | 'success' = typeof opts.start === 'number' || opts.start === 'success' ? opts.start : 0
  let dialog: HTMLDialogElement | null = null
  let teaserBtn: HTMLButtonElement | null = null
  const card = el('div', 'card rel')

  const currentStep = (): Step | undefined => (stepIndex === 'success' ? doc.success : doc.steps[stepIndex])

  const close = () => {
    emit('closed', { step: stepIndex })
    if (opts.mode === 'preview') {return}
    dialog?.close()
    if (doc.format !== 'popup') {card.parentElement?.remove()}
    if (doc.teaser && doc.teaser.show !== 'before' && stepIndex !== 'success') {showTeaser()}
  }

  const advance = (action: string) => {
    const inputs = card.querySelectorAll('input')
    inputs.forEach((i) => {
      if (i.name) {values[i.name] = i.value}
    })
    if (action === 'next' && typeof stepIndex === 'number' && stepIndex < doc.steps.length - 1) {
      stepIndex += 1
      emit('step', { step: stepIndex, values: { ...values } })
    } else {
      emit('submitted', { values: { ...values } })
      stepIndex = doc.success ? 'success' : stepIndex
      if (!doc.success) {close()}
    }
    paint()
  }

  const block = (b: Block): Node | null => {
    if (opts.device === 'mobile' && b.mobile?.hidden) {return null}
    const p = propsFor(b, opts.device)
    switch (b.type) {
      case 'heading': {
        const h = el('h2', '', String(p.text ?? ''))
        h.style.fontSize = `${Number(p.size ?? 22)}px`
        return h
      }
      case 'text':
        return el('p', '', String(p.text ?? ''))
      case 'image': {
        const img = el('img')
        img.src = String(p.src ?? '')
        img.alt = String(p.alt ?? '')
        return img
      }
      case 'email': {
        const l = el('label', '', String(p.label ?? 'Email'))
        const i = el('input')
        i.type = 'email'
        i.name = 'email'
        i.autocomplete = 'email'
        i.placeholder = String(p.placeholder ?? '')
        i.required = Boolean(p.required)
        l.append(i)
        return l
      }
      case 'phone-sms': {
        const box = el('div', 'col')
        const token = 'tok_demo123'
        const body = `${String(p.keyword ?? 'START')} ${token}`
        if (opts.device === 'mobile') {
          const a = el('a', 'btn', `Text ${String(p.keyword ?? 'START')} to join`)
          a.href = `sms:${String(p.number ?? '')}?&body=${encodeURIComponent(body)}`
          a.addEventListener('click', () => emit('clicked', { element: 'phone-sms' }))
          box.append(a)
        } else {
          box.append(qr(body), el('p', 'small', `Scan with your phone to text ${String(p.keyword ?? 'START')} to ${String(p.number ?? '')}`))
        }
        box.append(el('p', 'small', String(p.disclosure ?? '')))
        return box
      }
      case 'push-button': {
        const permission = typeof Notification === 'undefined' ? 'unsupported' : Notification.permission
        // ADR 0092: hidden once the browser has allowed or blocked; the preview shows it with a note.
        if (permission !== 'default' && opts.mode === 'live') {return null}
        const box = el('div', 'col')
        const btn = el('button', 'btn link', String(p.label ?? 'Get notifications'))
        btn.type = 'button'
        btn.addEventListener('click', () => {
          emit('clicked', { element: 'push-button' })
          // A click inside the shadow root is a user gesture in the page's own document.
          void Notification.requestPermission().then((r) => {
            btn.textContent = `Permission: ${r}`
          })
        })
        box.append(btn)
        if (permission !== 'default') {box.append(el('span', 'note', `(preview: hidden live, permission is ${permission})`))}
        return box
      }
      case 'button': {
        const action = String(p.action ?? 'submit')
        const btn = el('button', `btn${p.variant === 'link' ? ' link' : ''}`, String(p.label ?? 'Submit'))
        btn.type = 'button'
        btn.addEventListener('click', () => {
          emit('clicked', { element: 'button', action })
          if (action === 'close') {close()}
          else if (action === 'url') {window.open(String(p.url ?? '#'), '_blank')}
          else {advance(action)}
        })
        return btn
      }
      case 'columns': {
        const cols = el('div', 'cols')
        for (const side of [b.left ?? [], b.right ?? []]) {
          const col = el('div', 'col')
          side.forEach((child) => {
            const n = block(child)
            if (n) {col.append(n)}
          })
          cols.append(col)
        }
        return cols
      }
      default:
        return null
    }
  }

  function paint() {
    card.replaceChildren()
    if (doc.format !== 'embedded') {
      const x = el('button', 'x', '×')
      x.type = 'button'
      x.setAttribute('aria-label', 'Close')
      x.addEventListener('click', close)
      card.append(x)
    }
    currentStep()?.blocks.forEach((b) => {
      const n = block(b)
      if (n) {card.append(n)}
    })
  }

  function showTeaser() {
    if (!doc.teaser || teaserBtn) {return}
    teaserBtn = el('button', 'teaser', doc.teaser.text)
    teaserBtn.type = 'button'
    teaserBtn.addEventListener('click', () => {
      teaserBtn?.remove()
      teaserBtn = null
      open()
    })
    wrap.append(teaserBtn)
  }

  function open() {
    paint()
    emit('shown', { device: opts.device })
    if (opts.mode === 'preview') {return}
    if (doc.format === 'popup') {
      dialog = el('dialog')
      dialog.setAttribute('aria-label', doc.name)
      dialog.append(card)
      // Esc closes a modal dialog by itself; record it as a close.
      dialog.addEventListener('cancel', () => close())
      wrap.append(dialog)
      dialog.showModal()
    } else {
      const shell = el('div', doc.format === 'slide-in' ? 'slide' : doc.format === 'bar' ? 'bar' : '')
      shell.append(card)
      wrap.append(shell)
    }
  }

  if (opts.mode === 'preview') {
    if (opts.start === 'teaser' && doc.teaser) {
      wrap.classList.add('preview-teaser')
      showTeaser()
    } else {
      paint()
      if (doc.format === 'popup') {
        const overlay = el('div', 'preview-overlay')
        overlay.append(card)
        wrap.append(overlay)
      } else {
        wrap.append(card)
      }
    }
    opts.container?.append(host)
  } else {
    ;(opts.container ?? document.body).append(host)
    // ADR 0089: on a phone a popup opens from its teaser by default.
    const teaserFirst = doc.teaser && (doc.teaser.show !== 'after' || (doc.format === 'popup' && opts.device === 'mobile'))
    if (doc.format === 'popup' && teaserFirst) {showTeaser()}
    else {open()}
  }

  return { host, destroy: () => host.remove() }
}
