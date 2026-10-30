// A span of a server message: prose, or a command the message sets in backticks
// ("Run `sphericon migrate`"), which a page shows as code the user can copy.
export interface MessageSpan {
  text: string
  code: boolean
}

// splitInlineCode cuts a message at its backtick pairs. A backtick with no partner stays prose,
// and so does an empty pair.
export function splitInlineCode(message: string): MessageSpan[] {
  const spans: MessageSpan[] = []
  let rest = 0
  for (const match of message.matchAll(/`([^`]+)`/g)) {
    if (match.index > rest) {
      spans.push({ text: message.slice(rest, match.index), code: false })
    }
    spans.push({ text: match[1] ?? '', code: true })
    rest = match.index + match[0].length
  }
  if (rest < message.length) {
    spans.push({ text: message.slice(rest), code: false })
  }
  return spans
}
