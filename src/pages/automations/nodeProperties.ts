// The properties a Step's node holds on the canvas, by step: the builder's form holds every value
// as a string ('' the default), but for a Wait's seconds. The converters write them from a step and
// read them back, and the controls write the siblings of their own property, each by these names,
// so a misspelt one fails to compile instead of being saved as nothing.

// A switch a node turns off: OFF when it is off, '' when it is on (the default).
export const OFF = 'off'
export type Switch = '' | typeof OFF

export interface NodeProperties {
  email: { integrationId: string; subject: string; body: string; doc: string }
  telegram: {
    integrationId: string
    text: string
    unsubscribeButton: string
    imageUrl: string
    buttons: string
    linkPreview: Switch
  }
  sms: { integrationId: string; text: string; linkTracking: Switch }
  http: { stepId: string; url: string; headers: string; body: string; secret: string }
  wait: { seconds: number }
  tag: { tag: string }
}

// The properties of a node as read back: any of them may be missing or of another type.
export type ReadProperties<K extends keyof NodeProperties> = Partial<
  Record<keyof NodeProperties[K], unknown>
>

// The setter a control writes its node's sibling properties through.
export function propertySetter<K extends keyof NodeProperties>(control: {
  handleChange: (path: string, value: unknown) => void
}) {
  return <P extends keyof NodeProperties[K] & string>(name: P, value: NodeProperties[K][P]) =>
    control.handleChange(name, value)
}
