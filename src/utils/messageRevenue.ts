import type { SiteMessageRevenue } from '../generated/site/types.gen.ts'
import { formatMoney } from './money.ts'

// The net revenue credited to a Broadcast or an Automation, formatted in the Reporting currency.
// Null while the currency is unset: there is no figure to show then, not a zero.
export function formatMessageRevenue(revenue: SiteMessageRevenue): string | null {
  if (revenue.reportingCurrency === null) {
    return null
  }
  return formatMoney(revenue.amount, revenue.reportingCurrency)
}

// The Orders and Revenue cells of a list row: blank while the Reporting currency is unset (or the
// row has no revenue), so a table never shows a zero it did not count.
export function messageRevenueCells(revenue: SiteMessageRevenue | undefined): {
  orders: number | ''
  revenue: string
} {
  const formatted = revenue ? formatMessageRevenue(revenue) : null
  if (!revenue || formatted === null) {
    return { orders: '', revenue: '' }
  }
  return { orders: revenue.orders, revenue: formatted }
}
