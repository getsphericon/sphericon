package resources

import (
	"encoding/json"
	"fmt"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SPIKE (#1674): the per-place trees pass goverter through these extends, by their JSON: the sum
// types ogen makes of a union are switched on by hand or re-decoded, never mapped field by field.

func spikeSegmentConditionIn(c siteapi.SegmentConditionInput) (json.RawMessage, error) {
	b, err := c.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("encode segment condition: %w", err)
	}
	return b, nil
}

func spikeRuleConditionIn(c siteapi.RuleConditionInput) (json.RawMessage, error) {
	b, err := c.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("encode rule condition: %w", err)
	}
	return b, nil
}

// spikeSegmentCondition and spikeRuleCondition read a stored tree back into the response type; a
// tree that no longer decodes reads as the zero group.
func spikeSegmentCondition(raw json.RawMessage) siteapi.SegmentCondition {
	var c siteapi.SegmentCondition
	if err := c.UnmarshalJSON(raw); err != nil {
		return c
	}
	return c
}

func spikeRuleCondition(raw json.RawMessage) siteapi.RuleCondition {
	var c siteapi.RuleCondition
	if err := c.UnmarshalJSON(raw); err != nil {
		return c
	}
	return c
}
