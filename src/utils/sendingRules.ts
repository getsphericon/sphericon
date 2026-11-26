import type { SendingWindowChannel } from '../generated/site/types.gen.ts'
import { zSendingWindowChannel } from '../generated/site/zod.gen.ts'

// Whether a Sending window holds marketing messages on the Channel (ADR 0134), as the contract names
// the Channels it holds (SendingWindowChannel): a phone Channel's, whose number says which zones its
// recipient may be in. Email and Telegram go out at any hour.
export function hasSendingWindow(channel: string): channel is SendingWindowChannel {
  return zSendingWindowChannel.safeParse(channel).success
}

// Whether a cap or a window may be set for a country, by the number's country (ADR 0134, ADR
// 0137): only on a phone Channel, the one a Sending window holds.
export function takesCountry(channel: string): channel is SendingWindowChannel {
  return hasSendingWindow(channel)
}
