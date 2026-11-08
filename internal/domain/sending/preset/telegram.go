package preset

import (
	"encoding/json"
	"fmt"
)

// validateTelegramUpdate checks the provider identity before the transform can stringify it.
// Other Presets keep their own input and transform-failure policies.
func validateTelegramUpdate(body any) error {
	update, ok := body.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: telegram update is not an object", ErrMalformed)
	}
	number, ok := update["update_id"].(json.Number)
	if !ok {
		return fmt.Errorf("%w: telegram update_id is not an integer", ErrMalformed)
	}
	id, err := number.Int64()
	if err != nil || id < 0 {
		return fmt.Errorf("%w: telegram update_id is not a nonnegative integer", ErrMalformed)
	}
	return nil
}
