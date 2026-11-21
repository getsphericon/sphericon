package resources

import (
	"encoding/json"
	"fmt"

	externalapi "github.com/getsphericon/sphericon/gen/external"
)

// SPIKE (#1674): the per-place trees pass goverter through these extends, by their JSON: the sum
// types ogen makes of a union are switched on by hand or re-decoded, never mapped field by field.

func spikeSegmentConditionIn(c externalapi.SegmentConditionInput) (json.RawMessage, error) {
	b, err := c.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("encode segment condition: %w", err)
	}
	return b, nil
}

func spikeRuleConditionIn(c externalapi.RuleConditionInput) (json.RawMessage, error) {
	b, err := c.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("encode rule condition: %w", err)
	}
	return b, nil
}

// spikeSegmentCondition and spikeRuleCondition read a stored tree back into the response type; a
// tree that no longer decodes reads as the zero group.
func spikeSegmentCondition(raw json.RawMessage) externalapi.SegmentCondition {
	var c externalapi.SegmentCondition
	if err := c.UnmarshalJSON(raw); err != nil {
		return c
	}
	return c
}

func spikeRuleCondition(raw json.RawMessage) externalapi.RuleCondition {
	var c externalapi.RuleCondition
	if err := c.UnmarshalJSON(raw); err != nil {
		return c
	}
	return c
}
