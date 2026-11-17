package integrations

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/cleanup"
)

// ConfirmSubscription performs a Preset's confirm action: it fetches the URL the
// provider asked to be fetched, through the module's Confirmer. The engine has already
// refused a URL the provider's own rules do not allow (SNS: an https SNS host only).
func (m *Module) ConfirmSubscription(ctx context.Context, url string) error {
	return m.confirm.Confirm(ctx, url)
}

// Confirmer performs a Preset's confirm action on a URL the engine has already checked.
type Confirmer interface {
	Confirm(ctx context.Context, url string) error
}

// WebConfirmer is the Confirmer of an Instance: a GET through the SSRF-guarded client.
type WebConfirmer struct{}

// NewWebConfirmer builds the Confirmer the wiring hands New.
func NewWebConfirmer() *WebConfirmer { return &WebConfirmer{} }

// Confirm fetches url and drains its answer; a status other than 2xx is an error.
func (*WebConfirmer) Confirm(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := webhook.NewClient(10 * time.Second).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, drainErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	cleanup.Report("read the confirmation answer", drainErr)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("confirm subscription: status %d", resp.StatusCode)
	}
	return nil
}
