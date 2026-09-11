// SPDX-License-Identifier: MIT
package opencodefamily

import (
	"encoding/json"
	"errors"
)

// Native summaries are a discriminated field: assistant compaction markers are
// booleans, user summaries are diff objects, and session summaries are counters.
// Only the assistant marker participates in answer projection.
func (m *nativeInfo) UnmarshalJSON(data []byte) error {
	type fields nativeInfo
	var value fields
	wire := struct {
		*fields
		Summary json.RawMessage `json:"summary"`
	}{fields: &value}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if len(wire.Summary) != 0 {
		switch value.Role {
		case "assistant":
			if string(wire.Summary) != "true" && string(wire.Summary) != "false" {
				return errors.New("invalid native assistant summary")
			}
			value.Summary = string(wire.Summary) == "true"
		case "user":
			if err := validateNativeSummary(wire.Summary, true); err != nil {
				return err
			}
		default:
			return errors.New("native summary without message role")
		}
	}
	*m = nativeInfo(value)
	return nil
}

func validateNativeSummary(raw json.RawMessage, user bool) error {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return errors.New("invalid native summary object")
	}
	if user {
		for _, key := range []string{"title", "body"} {
			if value, ok := object[key]; ok {
				var text *string
				if json.Unmarshal(value, &text) != nil || text == nil {
					return errors.New("invalid native user summary text")
				}
			}
		}
	} else {
		for _, key := range []string{"additions", "deletions", "files"} {
			var number *float64
			if json.Unmarshal(object[key], &number) != nil || number == nil {
				return errors.New("invalid native session summary counters")
			}
		}
	}
	value, hasDiffs := object["diffs"]
	if !hasDiffs && !user {
		return nil
	}
	var diffs []struct {
		File      *string  `json:"file"`
		Patch     *string  `json:"patch"`
		Additions *float64 `json:"additions"`
		Deletions *float64 `json:"deletions"`
		Status    *string  `json:"status"`
	}
	if json.Unmarshal(value, &diffs) != nil || diffs == nil {
		return errors.New("invalid native summary diffs")
	}
	for _, diff := range diffs {
		if diff.Additions == nil || diff.Deletions == nil || (diff.Status != nil && *diff.Status != "added" && *diff.Status != "deleted" && *diff.Status != "modified") {
			return errors.New("invalid native summary diff")
		}
	}
	return nil
}
