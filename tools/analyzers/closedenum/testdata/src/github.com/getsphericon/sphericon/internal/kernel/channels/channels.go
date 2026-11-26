// Package channels stands in for the kernel's Channels.
package channels

type Channel string

const (
	Email    Channel = "email"
	SMS      Channel = "sms"
	Telegram Channel = "telegram"
)
