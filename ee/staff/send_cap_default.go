package staff

import (
	"context"
	"log/slog"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/instance"
)

// DefaultSendCap is the Instance's default Send cap (ADR 0067), nil when there is none;
// ErrNoSendCaps on an Instance without Send caps.
func (m *Module) DefaultSendCap(ctx context.Context) (*int, error) {
	if err := m.sendCapsOn(); err != nil {
		return nil, err
	}
	return instance.DefaultSendCap(ctx, m.ent)
}

// SetDefaultSendCap sets the Instance's default Send cap, or removes it when sendCap is nil, as
// the Staff member. Only a Workspace created afterwards gets it. The Instance has no Audit log
// of its own, so a change is recorded as a structured log line with the Staff member and the old
// and new values. It returns the default now and whether it changed.
func (m *Module) SetDefaultSendCap(ctx context.Context, by *ent.StaffMember, sendCap *int) (now *int, changed bool, err error) {
	if err := m.sendCapsOn(); err != nil {
		return nil, false, err
	}
	old, changed, err := instance.SetDefaultSendCap(ctx, m.ent, sendCap)
	if err != nil || !changed {
		return old, false, err
	}
	m.log.InfoContext(ctx, "staff changed the default send cap",
		"staff_member_id", by.ID, "from", capValue(old), "to", capValue(sendCap))
	return sendCap, true, nil
}

// sendCapsOn refuses a Send cap operation on an unlicensed Instance or one without Send caps.
func (m *Module) sendCapsOn() error {
	if err := m.licensed(); err != nil {
		return err
	}
	if !m.sendCaps {
		return ErrNoSendCaps
	}
	return nil
}

// capValue is a Send cap as a log value: the number, or nil for none.
func capValue(v *int) slog.Value {
	if v == nil {
		return slog.AnyValue(nil)
	}
	return slog.IntValue(*v)
}
