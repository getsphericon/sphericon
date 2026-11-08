package preset_test

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
)

func TestTelegramIdentityAcceptsExactlyNonnegativeIntegers(t *testing.T) {
	registry, err := preset.Load(nil)
	require.NoError(t, err)
	p, ok := registry.Get("telegram")
	require.True(t, ok)
	header := http.Header{}
	header.Set("X-Telegram-Bot-Api-Secret-Token", "property-secret")
	ctx := t.Context()
	rapid.Check(t, func(t *rapid.T) {
		id := rapid.Int64().Draw(t, "update_id")
		body := fmt.Sprintf(`{"update_id":%d,"message":{"chat":{"id":42,"type":"private"},"from":{"id":42},"text":"/stop"}}`, id)
		res, err := p.Handle(ctx, preset.Request{Header: header, Body: []byte(body)}, preset.Params{"secret": "property-secret"}, time.Unix(0, 0))
		malformed := errors.Is(err, preset.ErrMalformed)
		wantMalformed := id < 0
		assert.Equal(t, wantMalformed, malformed)
		wantFacts := lo.Ternary(wantMalformed, 0, 1)
		require.Len(t, res.InboundMessages, wantFacts)
		wantID := strconv.FormatInt(id, 10)
		for _, message := range res.InboundMessages {
			assert.Equal(t, wantID, message.ID, "the provider identity must survive without loss")
		}
	})
}
