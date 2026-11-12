export const READ_SCOPE = 'contacts:read'
export const WRITE_SCOPE = 'contacts:write'
export const EVENTS_SCOPE = 'events:read'
export const withSpread = [...others, 'contacts:read', 'contacts:write', 'events:read'] as const
