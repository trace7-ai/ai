package contract

import (
	"encoding/json"
	"fmt"
)

func stringPtr(value string) *string {
	return &value
}

func requireString(raw any, field string) (string, error) {
	value, ok := raw.(string)
	if !ok || value == "" {
		return "", fmt.Errorf("%s must be a non-empty string", field)
	}
	return value, nil
}

func optionalString(raw any, field string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	value, err := requireString(raw, field)
	if err != nil {
		return nil, err
	}
	return stringPtr(value), nil
}

func toStringSlice(raw any, field string) ([]string, error) {
	if raw == nil {
		return []string{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list", field)
	}
	result := make([]string, 0, len(items))
	for index, item := range items {
		value, err := requireString(item, fmt.Sprintf("%s[%d]", field, index))
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func requireObject(raw any, field string) (map[string]any, error) {
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	return value, nil
}

func decodeJSONObject(raw []byte) (map[string]any, error) {
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	return body, nil
}
