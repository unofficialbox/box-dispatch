package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

var ErrConnectionConflict = errors.New("connection update conflicts with current state")

// MergeObservedConnectionUpdate applies only fields changed by this caller.
// A concurrent same-target refresh is preserved. A changed destination or
// conflicting write is rejected rather than restoring captured old settings.
func MergeObservedConnectionUpdate(next, current ConnectionSettings) (ConnectionSettings, error) {
	baseline := next.ObservedConnectionState
	if baseline == nil {
		next.ObservedConnectionState = nil
		return next, nil
	}
	if DeploymentTargetFingerprint(*baseline) != DeploymentTargetFingerprint(current) {
		return current, fmt.Errorf("Selected connections changed while this request was running. Check the connections and try again: %w", ErrConnectionConflict)
	}
	values := make([]map[string]any, 3)
	for index, settings := range []ConnectionSettings{*baseline, next, current.HydrateBoxConnections().HydrateSalesforceOrgs()} {
		data, err := json.Marshal(settings)
		if err != nil {
			return current, fmt.Errorf("connection update could not be prepared")
		}
		if err := json.Unmarshal(data, &values[index]); err != nil {
			return current, fmt.Errorf("connection update could not be prepared")
		}
	}
	merged, err := mergeObservedValue(values[0], values[1], values[2])
	if err != nil {
		return current, fmt.Errorf("Connections were updated by another request. Check the connections and try again: %w", ErrConnectionConflict)
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return current, fmt.Errorf("connection update could not be prepared")
	}
	var result ConnectionSettings
	if err := json.Unmarshal(data, &result); err != nil {
		return current, fmt.Errorf("connection update could not be prepared")
	}
	return result.HydrateBoxConnections().HydrateSalesforceOrgs(), nil
}

func mergeObservedValue(before, next, current any) (any, error) {
	if reflect.DeepEqual(before, next) {
		return current, nil
	}
	if reflect.DeepEqual(before, current) || reflect.DeepEqual(next, current) {
		return next, nil
	}
	b, bOK := before.(map[string]any)
	n, nOK := next.(map[string]any)
	c, cOK := current.(map[string]any)
	if bOK && nOK && cOK {
		result := make(map[string]any, len(c))
		for key, value := range c {
			result[key] = value
		}
		keys := make(map[string]struct{})
		for key := range b {
			keys[key] = struct{}{}
		}
		for key := range n {
			keys[key] = struct{}{}
		}
		for key := range keys {
			value, err := mergeObservedValue(b[key], n[key], c[key])
			if err != nil {
				return nil, err
			}
			if value == nil {
				delete(result, key)
			} else {
				result[key] = value
			}
		}
		return result, nil
	}
	if bs, ok := before.([]any); ok {
		if ns, ok := next.([]any); ok {
			if cs, ok := current.([]any); ok {
				return mergeObservedRecords(bs, ns, cs)
			}
		}
	}
	return nil, fmt.Errorf("conflicting connection update")
}

func mergeObservedRecords(before, next, current []any) ([]any, error) {
	records := func(values []any) (map[string]any, error) {
		result := map[string]any{}
		for _, value := range values {
			record, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("record identity is unavailable")
			}
			id, ok := record["id"].(string)
			if !ok || id == "" {
				return nil, fmt.Errorf("record identity is unavailable")
			}
			if _, exists := result[id]; exists {
				return nil, fmt.Errorf("duplicate record identity")
			}
			result[id] = value
		}
		return result, nil
	}
	b, err := records(before)
	if err != nil {
		return nil, err
	}
	n, err := records(next)
	if err != nil {
		return nil, err
	}
	c, err := records(current)
	if err != nil {
		return nil, err
	}
	merged, err := mergeObservedValue(b, n, c)
	if err != nil {
		return nil, err
	}
	result := []any{}
	values := merged.(map[string]any)
	for _, original := range append(current, next...) {
		id := original.(map[string]any)["id"].(string)
		if record, ok := values[id]; ok {
			result = append(result, record)
			delete(values, id)
		}
	}
	return result, nil
}
