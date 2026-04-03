package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func ParseModelResult(raw string, contentFormat string) (any, string, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, "", fmt.Errorf("model result must be a non-empty string")
	}
	switch contentFormat {
	case "structured":
		value, err := extractJSONValue(raw)
		if err != nil {
			return nil, "", err
		}
		return value, "structured", nil
	case "markdown", "text":
		return text, contentFormat, nil
	default:
		value, err := decodeAutoResult(text)
		if err == nil {
			return value, "structured", nil
		}
		return text, "text", nil
	}
}

func decodeAutoResult(raw string) (any, error) {
	value, err := decodeJSONDocument(raw)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func extractJSONValue(raw string) (any, error) {
	text := strings.TrimSpace(raw)
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 3 {
			text = strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		}
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		return decodeJSONDocument(text)
	}
	for index, char := range text {
		if char != '{' && char != '[' {
			continue
		}
		value, err := decodeJSONDocument(text[index:])
		if err == nil {
			return value, nil
		}
	}
	return nil, fmt.Errorf("model response did not contain a JSON value")
}

func decodeJSONDocument(raw string) (any, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var tail any
	if err := decoder.Decode(&tail); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("model response JSON must contain a single value")
		}
		return nil, err
	}
	return value, nil
}
