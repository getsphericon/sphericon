package commandline

import (
	"fmt"
	"io"

	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
)

// WritePresetTests reports `sphericon preset test` without a sample: PASS or FAIL for each case
// the Preset named name carries (preset.Preset.RunTests), with how a failed case disagreed. It
// fails when any case did.
func WritePresetTests(w io.Writer, name string, results []preset.CaseResult) error {
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
			_, _ = fmt.Fprintf(w, "FAIL %s\n     %v\n", r.Name, r.Err)
			continue
		}
		_, _ = fmt.Fprintf(w, "PASS %s\n", r.Name)
	}
	if failed > 0 {
		return fmt.Errorf("preset %s: %d of %d cases failed", name, failed, len(results))
	}
	return nil
}

// WritePresetSample reports `sphericon preset test --payload`: the facts a verified sample made
// (preset.Preset.Handle), one line each, or the reason it was refused, which it returns.
func WritePresetSample(w io.Writer, res preset.Result, err error) error {
	if err != nil {
		_, _ = fmt.Fprintf(w, "refused: %v\n", err)
		return err
	}
	for i := range res.Notices {
		n := &res.Notices[i]
		_, _ = fmt.Fprintf(w, "%s %s id=%s sending_domain=%s message_id=%s reason=%q channel=%s status=%s error_code=%s\n",
			noticeKind(n), n.Destination, n.ID, n.SendingDomain, n.MessageID, n.Reason, n.Channel, n.Status, n.ErrorCode)
	}
	for i := range res.InboundMessages {
		m := &res.InboundMessages[i]
		_, _ = fmt.Fprintf(w, "%s %s %s id=%s chat_id=%s message_id=%s text=%q payload=%q\n",
			preset.KindInboundMessage, m.Channel, m.Destination, m.ID, m.ChatID, m.MessageID, m.Text, m.Payload)
	}
	for _, c := range res.ReachabilityChanges {
		if c.Reachable {
			_, _ = fmt.Fprintf(w, "%s %s id=%s reachable\n", preset.KindReachabilityChange, c.Destination, c.ID)
			continue
		}
		_, _ = fmt.Fprintf(w, "%s %s id=%s unreachable reason=%s\n", preset.KindReachabilityChange, c.Destination, c.ID, c.Reason)
	}
	if res.Empty() {
		_, _ = fmt.Fprintln(w, "verified, no facts (an event the Preset does not handle)")
	}
	return nil
}

// noticeKind is the kind of a notice as printed: a bounce says whether it is hard or soft.
func noticeKind(n *preset.DeliveryNotice) string {
	if n.Kind != preset.Bounce {
		return string(n.Kind)
	}
	if n.Hard {
		return string(n.Kind) + " hard"
	}
	return string(n.Kind) + " soft"
}
