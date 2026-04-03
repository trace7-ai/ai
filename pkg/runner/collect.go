package runner

import (
	"context"
	"fmt"
	"io"
	"strings"

	"mira/pkg/contract"
	"mira/pkg/transport"
)

func collectStream(ctx context.Context, stream transport.Stream, onChunk func([]byte)) (string, contract.TokenUsage, bool, error) {
	chunks := []string{}
	hadReasonText := false
	usage := contract.TokenUsage{}
	stopReason := ""
	for {
		event, err := stream.Next(ctx)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", usage, false, err
		}
		switch event.Type {
		case "done":
			return strings.TrimSpace(strings.Join(chunks, "")), usage, stopReason == "max_tokens", nil
		case "usage":
			if event.Usage != nil {
				usage = *event.Usage
			}
			if event.StopReason != "" {
				stopReason = event.StopReason
			}
		case "content":
			if event.StopReason != "" {
				stopReason = event.StopReason
			}
			if event.FromContent && hadReasonText {
				continue
			}
			if !event.FromContent {
				hadReasonText = true
			}
			if onChunk != nil {
				onChunk([]byte(event.Text))
			}
			chunks = append(chunks, event.Text)
		default:
			return "", usage, false, fmt.Errorf("unknown stream event: %s", event.Type)
		}
	}
	return "", usage, false, fmt.Errorf("incomplete stream")
}
