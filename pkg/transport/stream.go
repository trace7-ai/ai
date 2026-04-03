package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"mira/pkg/contract"
)

type SSEStream struct {
	response         *http.Response
	reader           *bufio.Reader
	completedCleanly bool
}

func NewSSEStream(response *http.Response) *SSEStream {
	return &SSEStream{
		response: response,
		reader:   bufio.NewReader(response.Body),
	}
}

func (stream *SSEStream) Next(ctx context.Context) (Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Event{}, err
		}
		line, err := stream.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if stream.completedCleanly {
					return Event{Type: "done"}, nil
				}
				return Event{}, fmt.Errorf("stream ended unexpectedly")
			}
			if ctx.Err() != nil {
				return Event{}, ctx.Err()
			}
			return Event{}, fmt.Errorf("stream read failed: %v", err)
		}
		payload, ok := decodeSSEPayload(line)
		if !ok {
			continue
		}
		event, err := stream.eventFromPayload(payload)
		if err != nil {
			return Event{}, err
		}
		if event.Type != "" {
			return event, nil
		}
	}
}

func (stream *SSEStream) Close() error {
	return stream.response.Body.Close()
}

func decodeSSEPayload(line string) (map[string]any, bool) {
	text := trimLine(line)
	if text == "" || text == ":keep-alive" {
		return nil, false
	}
	if len(text) >= 6 && text[:6] == "data: " {
		text = text[6:]
	} else if len(text) >= 5 && text[:5] == "data:" {
		text = text[5:]
	} else {
		return nil, false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil, false
	}
	return payload, true
}

func (stream *SSEStream) eventFromPayload(raw map[string]any) (Event, error) {
	if done, _ := raw["done"].(bool); done {
		stream.completedCleanly = true
		return Event{Type: "done"}, nil
	}
	if errorValue, ok := raw["error"]; ok {
		if body, ok := errorValue.(map[string]any); ok {
			if message, _ := body["message"].(string); message != "" {
				return Event{}, fmt.Errorf("%s", message)
			}
		}
		return Event{}, fmt.Errorf("%v", errorValue)
	}
	message := raw["Message"]
	if message == nil {
		if code, _ := raw["code"].(float64); code == 0 {
			stream.completedCleanly = true
			return Event{Type: "done"}, nil
		}
		return Event{}, nil
	}
	messageBody, err := normalizeMessage(message)
	if err != nil {
		return Event{}, nil
	}
	event, _ := messageBody["event"].(string)
	data, _ := messageBody["data"].(map[string]any)
	if event == "reason" {
		return reasonEvent(data)
	}
	if event == "content" {
		return contentEvent(data), nil
	}
	return Event{}, nil
}

func normalizeMessage(raw any) (map[string]any, error) {
	switch value := raw.(type) {
	case string:
		var body map[string]any
		if err := json.Unmarshal([]byte(value), &body); err != nil {
			return nil, err
		}
		return body, nil
	case map[string]any:
		return value, nil
	default:
		return nil, fmt.Errorf("unexpected message type")
	}
}

func reasonEvent(data map[string]any) (Event, error) {
	inner, _ := data["event"].(map[string]any)
	if inner == nil {
		return Event{}, nil
	}
	innerType, _ := inner["type"].(string)
	switch innerType {
	case "content_block_delta":
		delta, _ := inner["delta"].(map[string]any)
		if delta == nil {
			return Event{}, nil
		}
		deltaType, _ := delta["type"].(string)
		if deltaType == "input_json_delta" {
			return Event{}, nil
		}
		text, _ := delta["text"].(string)
		if text == "" {
			return Event{}, nil
		}
		return Event{Type: "content", Text: text}, nil
	case "message_delta":
		usage, _ := inner["usage"].(map[string]any)
		return Event{Type: "usage", Usage: parseUsage(usage), StopReason: deltaStopReason(inner)}, nil
	default:
		return Event{}, nil
	}
}

func contentEvent(data map[string]any) Event {
	body := resultPayload(data)
	if body == nil {
		return Event{}
	}
	text, _ := body["text"].(string)
	if text == "" {
		text, _ = body["result"].(string)
	}
	if text == "" {
		return Event{}
	}
	return Event{Type: "content", Text: text, FromContent: true, StopReason: fieldStopReason(body)}
}

func parseUsage(raw map[string]any) *contract.TokenUsage {
	if raw == nil {
		return nil
	}
	usage := &contract.TokenUsage{}
	if input, ok := raw["input_tokens"].(float64); ok {
		value := int(input)
		usage.Input = &value
	}
	if output, ok := raw["output_tokens"].(float64); ok {
		value := int(output)
		usage.Output = &value
	}
	return usage
}

func deltaStopReason(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	delta, _ := raw["delta"].(map[string]any)
	if delta == nil {
		return ""
	}
	stopReason, _ := delta["stop_reason"].(string)
	return stopReason
}

func fieldStopReason(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	stopReason, _ := raw["stop_reason"].(string)
	return stopReason
}

func resultPayload(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}
	if eventType, _ := data["type"].(string); eventType == "result" {
		return data
	}
	body, _ := data["content"].(map[string]any)
	if body == nil {
		return nil
	}
	if eventType, _ := body["type"].(string); eventType != "result" {
		return nil
	}
	return body
}

func trimLine(line string) string {
	start := 0
	end := len(line)
	for start < end && (line[start] == '\n' || line[start] == '\r') {
		start++
	}
	for end > start && (line[end-1] == '\n' || line[end-1] == '\r') {
		end--
	}
	return line[start:end]
}
