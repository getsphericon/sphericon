import { Typography } from '@mantine/core'
import Markdown from 'react-markdown'

// LegalMarkdown renders a Legal document's Markdown (ADR 0062), styled by Mantine's Typography.
// The text is written by a Staff member; raw HTML in it is not rendered, only the Markdown.
export function LegalMarkdown({ markdown }: { markdown: string }) {
  return (
    <Typography>
      <Markdown>{markdown}</Markdown>
    </Typography>
  )
}
