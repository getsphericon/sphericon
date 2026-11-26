// Package medium declares types derived from channels.Channel, for packages that switch on them.
package medium

import "github.com/getsphericon/sphericon/internal/kernel/channels"

// Deeper is declared before the type it derives from.
type Deeper Medium // want Deeper:"derived from channels.Channel"

type Medium channels.Channel // want Medium:"derived from channels.Channel"

type Alias = channels.Channel

const (
	Email    = Medium(channels.Email)
	SMS      = Medium(channels.SMS)
	Telegram = Medium(channels.Telegram)
)
