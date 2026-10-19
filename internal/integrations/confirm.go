package integrations

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/internal/webhook"
)

// ConfirmSubscription performs a Preset's confirm action: it fetches the URL the
// provider asked to be fetched, through the SSRF-guarded client. The engine has already
// refused a URL the provider's own rules do not allow (SNS: an https SNS host only).
func (m *Module) ConfirmSubscription(ctx context.Context, url string) error {
	return m.confirm(ctx, url)
}

func confirmByGet(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := webhook.NewClient(10 * time.Second).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("confirm subscription: status %d", resp.StatusCode)
	}
	return nil
}
