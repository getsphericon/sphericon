// PROTOTYPE (throwaway, branch prototype/onsite-form-builder, ticket #958). Not production code.
//
// The one stored document every builder variant writes and the one renderer reads. It is a closed
// schema: a format fixed at creation (ADR 0085), a theme copied in at authoring (ADR 0003, 0085),
// linear steps of blocks, an optional success step and a teaser. A block may carry a mobile
// override: "one design, linked per-device overrides" (research/on-site-builders).

export type Format = 'popup' | 'slide-in' | 'bar' | 'embedded'
export type Device = 'desktop' | 'mobile'

export interface Theme {
  primary: string
  background: string
  text: string
  font: string
  radius: number
}

export type BlockType =
  | 'heading'
  | 'text'
  | 'image'
  | 'email'
  | 'phone-sms'
  | 'push-button'
  | 'button'
  | 'columns'

export interface Block {
  id: string
  type: BlockType
  props: Record<string, unknown>
  // Only what differs on a phone. Absent = linked to desktop.
  mobile?: { hidden?: boolean; props?: Record<string, unknown> }
  // columns only: two child lists
  left?: Block[]
  right?: Block[]
}

export interface Step {
  id: string
  blocks: Block[]
}

export interface FormDoc {
  schema: 1
  id: string
  name: string
  format: Format
  placement?: string
  theme: Theme
  steps: Step[]
  success?: Step
  teaser?: { text: string; show: 'before' | 'after' | 'both' }
}

let seq = 0
export const uid = (prefix: string) => `${prefix}_${Date.now().toString(36)}${(seq++).toString(36)}`

// "Detected from the site": a palette the author picks from; it is copied, never referenced.
export const SITE_PALETTE: Theme = {
  primary: '#228be6',
  background: '#ffffff',
  text: '#1a1b1e',
  font: 'system-ui, -apple-system, Segoe UI, Roboto, sans-serif',
  radius: 8,
}

export function samplePopup(): FormDoc {
  return {
    schema: 1,
    id: 'form_popup',
    name: 'Welcome popup',
    format: 'popup',
    theme: { ...SITE_PALETTE },
    steps: [
      {
        id: 'step_1',
        blocks: [
          { id: 'b1', type: 'image', props: { src: 'https://picsum.photos/seed/brightpath/480/200', alt: '' }, mobile: { hidden: true } },
          { id: 'b2', type: 'heading', props: { text: 'Get 10% off your first course', size: 26 }, mobile: { props: { size: 20 } } },
          { id: 'b3', type: 'text', props: { text: 'Join the Brightpath list for new courses and study tips.' } },
          { id: 'b4', type: 'email', props: { label: 'Email', placeholder: 'you@example.com', required: true } },
          { id: 'b5', type: 'button', props: { label: 'Continue', action: 'next' } },
        ],
      },
      {
        id: 'step_2',
        blocks: [
          { id: 'b6', type: 'heading', props: { text: 'Want texts too?', size: 22 } },
          {
            id: 'b7',
            type: 'phone-sms',
            props: {
              keyword: 'START',
              number: '+15555550123',
              disclosure:
                'By texting START you agree to receive recurring marketing texts from Brightpath. Msg & data rates may apply. Up to 4 msgs/month. Reply STOP to cancel, HELP for help. Terms: brightpath.example/terms',
            },
          },
          { id: 'b8', type: 'button', props: { label: 'No thanks', action: 'submit', variant: 'link' } },
        ],
      },
    ],
    success: {
      id: 'success',
      blocks: [
        { id: 'b9', type: 'heading', props: { text: 'Check your inbox', size: 22 } },
        { id: 'b10', type: 'text', props: { text: 'Your code is on its way.' } },
        { id: 'b11', type: 'button', props: { label: 'Close', action: 'close' } },
      ],
    },
    teaser: { text: '10% off', show: 'after' },
  }
}

export function sampleEmbedded(): FormDoc {
  return {
    schema: 1,
    id: 'form_footer',
    name: 'Article footer signup',
    format: 'embedded',
    placement: 'article-footer',
    theme: { ...SITE_PALETTE, background: '#f1f3f5', radius: 4 },
    steps: [
      {
        id: 'step_1',
        blocks: [
          {
            id: 'c1',
            type: 'columns',
            props: {},
            left: [
              { id: 'c2', type: 'heading', props: { text: 'Liked this article?', size: 20 } },
              { id: 'c3', type: 'text', props: { text: 'One email a week with the best of Brightpath.' } },
            ],
            right: [
              { id: 'c4', type: 'email', props: { label: 'Email', placeholder: 'you@example.com', required: true } },
              { id: 'c5', type: 'button', props: { label: 'Subscribe', action: 'submit' } },
              { id: 'c6', type: 'push-button', props: { label: 'Or get browser notifications' } },
            ],
          },
        ],
      },
    ],
    success: { id: 'success', blocks: [{ id: 'c7', type: 'text', props: { text: 'Thanks, you are in.' } }] },
  }
}

// Same-origin storage stands in for the Form host's config (ADR 0090). PROTOTYPE, wipe me.
const KEY = 'PROTOTYPE_onsite_forms'

export function loadDocs(): FormDoc[] {
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) {
      return JSON.parse(raw) as FormDoc[]
    }
  } catch {
    // fall through
  }
  return [samplePopup(), sampleEmbedded()]
}

export function saveDoc(doc: FormDoc): void {
  const docs = loadDocs().filter((d) => d.id !== doc.id)
  localStorage.setItem(KEY, JSON.stringify([...docs, doc]))
}

export function resetDocs(): void {
  localStorage.removeItem(KEY)
}

// Props on this device: the mobile override wins over the desktop value.
export function propsFor(block: Block, device: Device): Record<string, unknown> {
  return device === 'mobile' && block.mobile?.props ? { ...block.props, ...block.mobile.props } : block.props
}
