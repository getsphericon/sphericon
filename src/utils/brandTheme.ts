import type { EmailEditorProps } from '@react-email/editor'

import type { SiteBrandKitResource } from '../generated/site/types.gen.ts'

export type EditorTheme = NonNullable<EmailEditorProps['theme']>

// The editor's own built-in theme, for a Workspace that has not saved a Brand kit.
const BASIC_THEME: EditorTheme = 'basic'

// The editor theme a Brand kit sets (ADR 0040): its colors, fonts and button style become the
// editor's styles. The editor writes them into the document of a message when the message is first
// authored, so what a message looks like is part of the message and a later change of the Brand kit
// never reaches it. A Workspace with no saved kit gets the editor's own theme.
export function brandTheme(kit: SiteBrandKitResource | undefined): EditorTheme {
  if (!kit?.saved) {
    return BASIC_THEME
  }
  const text = { color: kit.textColor, fontFamily: kit.bodyFont }
  const heading = { color: kit.textColor, fontFamily: kit.headingFont }
  return {
    extends: 'basic',
    styles: {
      body: { backgroundColor: kit.backgroundColor },
      container: { backgroundColor: kit.backgroundColor },
      paragraph: text,
      listItem: text,
      h1: heading,
      h2: heading,
      h3: heading,
      link: { color: kit.accentColor },
      button: {
        backgroundColor: kit.accentColor,
        color: kit.buttonTextColor,
        borderRadius: `${kit.buttonRadius}px`,
        fontFamily: kit.bodyFont,
      },
    },
  }
}
