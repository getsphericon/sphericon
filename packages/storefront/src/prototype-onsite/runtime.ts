// PROTOTYPE (throwaway, branch prototype/onsite-form-builder, ticket #958). Not production code.
//
// What the tracker would do on a customer's page: read the live forms (here from same-origin
// localStorage instead of the Form host, ADR 0090), open the overlay after a delay, fill every
// Placement the page reserves, and remember a close for the session (ADR 0092, 0093).

import type { Device, FormDoc } from './doc.ts'
import { loadDocs } from './doc.ts'
import { renderForm } from './render.ts'

const closedKey = (id: string) => `PROTOTYPE_closed_${id}`
const submittedKey = (id: string) => `PROTOTYPE_submitted_${id}`

export function startOnsite(device: () => Device, log: (line: string) => void, skip: () => boolean): () => void {
  let overlayShown = false
  const handlers = (doc: FormDoc) => (event: string, data?: Record<string, unknown>) => {
    log(`${doc.id} ${event} ${data ? JSON.stringify(data) : ''}`)
    if (event === 'closed') {sessionStorage.setItem(closedKey(doc.id), '1')}
    if (event === 'submitted') {localStorage.setItem(submittedKey(doc.id), '1')}
  }
  const eligible = (doc: FormDoc) => !sessionStorage.getItem(closedKey(doc.id)) && !localStorage.getItem(submittedKey(doc.id))

  const fillPlacements = () => {
    if (skip()) {return}
    for (const doc of loadDocs().filter((d) => d.format === 'embedded' && eligible(d))) {
      document.querySelectorAll(`[data-sphericon-placement="${doc.placement ?? ''}"]`).forEach((slot) => {
        if ((slot as HTMLElement).dataset.sphericonFilled) {return
        ;}(slot as HTMLElement).dataset.sphericonFilled = doc.id
        renderForm(doc, { device: device(), mode: 'live', container: slot as HTMLElement, onEvent: handlers(doc) })
      })
    }
  }

  const timer = window.setTimeout(() => {
    if (skip() || overlayShown) {return}
    // ADR 0092: one overlay per page view.
    const doc = loadDocs().find((d) => d.format !== 'embedded' && eligible(d))
    if (doc) {
      overlayShown = true
      renderForm(doc, { device: device(), mode: 'live', onEvent: handlers(doc) })
    }
  }, 1500)

  const observer = new MutationObserver(fillPlacements)
  observer.observe(document.body, { childList: true, subtree: true })
  fillPlacements()

  return () => {
    window.clearTimeout(timer)
    observer.disconnect()
  }
}

export function resetFormState(): void {
  for (const k of Object.keys(sessionStorage)) {if (k.startsWith('PROTOTYPE_closed_')) sessionStorage.removeItem(k)}
  for (const k of Object.keys(localStorage)) {if (k.startsWith('PROTOTYPE_submitted_')) localStorage.removeItem(k)}
}

// A site whose global CSS fights the form: a tiny root font size, a body font, global button and
// input rules. Applied through document.adoptedStyleSheets so a strict CSP does not block it.
const HOSTILE = `
html { font-size: 62.5% !important; }
body { font-family: "Comic Sans MS", "Comic Sans", cursive !important; color: #c2255c !important; line-height: 3 !important; letter-spacing: 2px !important; }
button { background: hotpink !important; border-radius: 0 !important; text-transform: uppercase !important; }
input { border: 3px dashed green !important; }
h2, p { margin: 40px !important; }
`
let hostile: CSSStyleSheet | null = null
export function setHostileCss(on: boolean): void {
  hostile ??= new CSSStyleSheet()
  hostile.replaceSync(HOSTILE)
  const others = document.adoptedStyleSheets.filter((s) => s !== hostile)
  document.adoptedStyleSheets = on ? [...others, hostile] : others
}
