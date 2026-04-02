package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"mira/pkg/roles"
)

func ParseModelResult(raw string, role roles.Spec, contentFormat string) (any, error) {
	if contentFormat != "structured" {
		text := strings.TrimSpace(raw)
		if text == "" {
			return nil, fmt.Errorf("model result must be a non-empty string")
		}
		return text, nil
	}
	value, err := extractJSONObject(raw)
	if err != nil {
		return nil, err
	}
	for _, key := range role.RequiredKeys {
		if _, ok := value[key]; !ok {
			return nil, fmt.Errorf("model result missing required key: %s", key)
		}
	}
	return value, nil
}

func extractJSONObject(raw string) (map[string]any, error) {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 3 {
			text = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	if strings.HasPrefix(text, "{") {
		return decodeJSONObject(text)
	}
	for index, char := range text {
		if char != '{' {
			continue
		}
		value, err := decodeJSONObject(text[index:])
		if err == nil {
			return value, nil
		}
	}
	return nil, fmt.Errorf("model response did not contain a JSON object")
}

func decodeJSONObject(raw string) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	body, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("model response JSON must be an object")
	}
	return body, nil
}
