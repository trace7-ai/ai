package roles

import "fmt"

const Assistant = "assistant"

func Normalize(name string) (string, error) {
	switch name {
	case "", Assistant, "planner", "reader", "reviewer":
		return Assistant, nil
	default:
		return "", fmt.Errorf("unsupported role: %s", name)
	}
}

func ResolveContentFormat(requested string) (string, error) {
	contentFormat := requested
	if contentFormat == "" {
		contentFormat = "auto"
	}
	switch contentFormat {
	case "auto", "structured", "markdown", "text":
		return contentFormat, nil
	default:
		return "", fmt.Errorf("unsupported content_format: %s", contentFormat)
	}
}
