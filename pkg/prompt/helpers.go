package prompt

import (
	"encoding/json"
)

func outputInstructions(contentFormat string, example string) string {
	switch contentFormat {
	case "structured":
		return "Return exactly one JSON object with no markdown fences.\nReturn only the result object matching this shape:\n" + example
	case "markdown":
		return "Return a direct markdown answer.\nUse headings, bullets, blockquotes, and code fences when helpful.\nDo not return JSON.\nDo not wrap the entire answer in a single code fence."
	default:
		return "Return direct plain text only. Do not return JSON."
	}
}

func mustPrettyJSON(value any) string {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(body)
}

func nullableString(value *string) string {
	if value == nil {
		return "none"
	}
	return *value
}
