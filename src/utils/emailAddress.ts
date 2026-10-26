// An address as its local part and domain; an address with no "@" is all local part.
export function splitEmail(email: string): { local: string; domain: string } {
  const at = email.lastIndexOf('@')
  return at < 0
    ? { local: email, domain: '' }
    : { local: email.slice(0, at), domain: email.slice(at + 1) }
}

// An empty local part is no address, so the result is empty.
export function joinEmail(local: string, domain: string): string {
  return local ? `${local}@${domain}` : ''
}
