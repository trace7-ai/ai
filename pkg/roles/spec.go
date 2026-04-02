package roles

import "fmt"

type Spec struct {
	Name                    string
	Summary                 string
	DefaultContentFormat    string
	SupportedContentFormats map[string]struct{}
	RequiredKeys            []string
	ResultExample           string
	ContextGuidance         string
	MarkdownStyle           string
	StructuredStyle         string
}

func (spec Spec) ResolveContentFormat(requested string) (string, error) {
	contentFormat := requested
	if contentFormat == "" || contentFormat == "auto" {
		contentFormat = spec.DefaultContentFormat
	}
	if _, ok := spec.SupportedContentFormats[contentFormat]; !ok {
		return "", fmt.Errorf("role does not support content_format=%s: %s", contentFormat, spec.Name)
	}
	return contentFormat, nil
}

func (spec Spec) OutputStyleGuidance(contentFormat string) string {
	if contentFormat == "structured" {
		return spec.StructuredStyle
	}
	return spec.MarkdownStyle
}
