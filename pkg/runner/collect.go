package runner

import (
	"context"
	"fmt"
	"io"
	"strings"

	"mira/pkg/contract"
	"mira/pkg/transport"
)

func collectStream(ctx context.Context, stream transport.Stream) (string, contract.TokenUsage, error) {
	chunks := []string{}
	hadReasonText := false
	usage := contract.TokenUsage{}
	for {
		event, err := stream.Next(ctx)
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", usage, err
		}
		switch event.Type {
		case "done":
			return strings.TrimSpace(strings.Join(chunks, "")), usage, nil
		case "usage":
			if event.Usage != nil {
				usage = *event.Usage
			}
		case "content":
			if event.FromContent && hadReasonText {
				continue
			}
			if !event.FromContent {
				hadReasonText = true
			}
			chunks = append(chunks, event.Text)
		default:
			return "", usage, fmt.Errorf("unknown stream event: %s", event.Type)
		}
	}
	return "", usage, fmt.Errorf("incomplete stream")
}
