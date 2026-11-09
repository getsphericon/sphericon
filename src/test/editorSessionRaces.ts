import { expect, test } from 'vitest'

// What the race table drives: one editor session (src/editing/editorSession.ts) over a stored
// resource, with an in-memory editor standing for the form or canvas a person types into, and a
// page standing for what the session's effects leave behind.
export interface RaceRig<TEvent extends { type: string }> {
  // Captures the current content and sends SAVE, as the editor's Save does.
  save: () => void
  canSave: () => boolean
  // Changes the current content, as a person typing does.
  edit: () => void
  // The adapter's other operations (Publish, Revert, …), each available while idle.
  operations: TEvent[]
  send: (event: TEvent) => void
  can: (event: TEvent) => boolean
  stop: () => void
  settled: () => Promise<{ saved: string; result: string; revision: number }>
  // The fingerprint of the current content and of the last submission.
  current: () => string
  submitted: () => string
  // What the effects left on the page: the resource in its cache, the toasts it showed
  // (`accepted SAVE`, `failed PUBLISH`, …), and the cached resource a refresh of the related
  // queries started over (null until one starts).
  page: { cached: unknown; toasts: string[]; refreshedOver: unknown }
  // The resource as the server holds it now, and as it was stored when the session opened.
  stored: () => Promise<unknown>
  initial: unknown
  // The first write waits at the gate until released; `answered` resolves once its handler has
  // returned a response.
  gate: { received: Promise<unknown>; release: () => void; answered: Promise<unknown> }
}

// The races every editor session must settle the same way, whatever its adapter. An adapter's test
// calls this with a rig over its own resource; `start` installs the msw handlers it needs.
export function editorSessionRaces<TEvent extends { type: string }>(
  start: (options: { refuse: boolean }) => RaceRig<TEvent>,
) {
  test('an edit during Save stays current and unsaved after the acknowledgement', async () => {
    const rig = start({ refuse: false })
    rig.save()
    const submitted = rig.submitted()
    await rig.gate.received
    rig.edit()
    const edited = rig.current()
    rig.gate.release()
    const settled = await rig.settled()
    const current = rig.current()
    const stored = await rig.stored()
    expect(settled.result).toBe('success')
    expect(settled.saved).toBe(submitted)
    expect(current).toBe(edited)
    expect(current).not.toBe(settled.saved)
    expect(settled.revision).toBe(0)
    expect(rig.page.cached).toEqual(stored)
    expect(rig.page.toasts).toEqual(['accepted SAVE'])
    // The acknowledgement is the write's success: the refresh starts over its cached resource.
    expect(rig.page.refreshedOver).toEqual(stored)
  })

  test('a refused Save keeps the acknowledged content and allows another attempt', async () => {
    const rig = start({ refuse: true })
    const { saved: before } = await rig.settled()
    rig.save()
    await rig.gate.received
    rig.gate.release()
    const settled = await rig.settled()
    const another = rig.canSave()
    expect(settled.result).toBe('error')
    expect(settled.saved).toBe(before)
    expect(settled.revision).toBe(0)
    expect(rig.page).toEqual({ cached: null, toasts: ['failed SAVE'], refreshedOver: null })
    expect(another).toBe(true)
    rig.save()
    await rig.settled()
    expect(rig.page.toasts).toEqual(['failed SAVE', 'failed SAVE'])
  })

  test('the other operations wait while Save is pending', async () => {
    const rig = start({ refuse: false })
    const allowedBefore = new Set(rig.operations.map((operation) => rig.can(operation)))
    rig.save()
    await rig.gate.received
    const allowedDuring = new Set(rig.operations.map((operation) => rig.can(operation)))
    for (const operation of rig.operations) {
      rig.send(operation)
    }
    rig.gate.release()
    await rig.settled()
    expect(allowedBefore).toEqual(new Set([true]))
    expect(allowedDuring).toEqual(new Set([false]))
    expect(rig.page.toasts).toEqual(['accepted SAVE'])
  })

  test('a Save answered after the session stopped changes nothing on the page', async () => {
    const rig = start({ refuse: false })
    const before = rig.current()
    rig.save()
    await rig.gate.received
    rig.stop()
    rig.gate.release()
    await rig.gate.answered
    // A later round trip through the server: the stopped session had the answer by now.
    const stored = await rig.stored()
    const current = rig.current()
    expect(stored).not.toEqual(rig.initial)
    expect(current).toBe(before)
    expect(rig.page).toEqual({ cached: null, toasts: [], refreshedOver: null })
  })
}
