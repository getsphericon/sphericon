package staff

import "github.com/getsphericon/sphericon/internal/login"

// UseThrottle swaps the failure counter the login runs on, to test a broken one.
func (m *Module) UseThrottle(t login.Throttle) { m.login = m.newLogin(t) }
