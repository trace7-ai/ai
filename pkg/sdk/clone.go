package sdk

import "mira/pkg/contract"

func cloneString(value string) *string {
	return &value
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	return cloneString(*value)
}

func cloneStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(items))
	return append(result, items...)
}

func cloneFiles(items []contract.ContextFile) []contract.ContextFile {
	if len(items) == 0 {
		return []contract.ContextFile{}
	}
	result := make([]contract.ContextFile, 0, len(items))
	for _, item := range items {
		result = append(result, contract.ContextFile{
			Path:    item.Path,
			Content: item.Content,
			Source:  cloneStringPtr(item.Source),
			Title:   cloneStringPtr(item.Title),
		})
	}
	return result
}

func cloneDocs(items []contract.ContextDoc) []contract.ContextDoc {
	if len(items) == 0 {
		return []contract.ContextDoc{}
	}
	result := make([]contract.ContextDoc, 0, len(items))
	for _, item := range items {
		result = append(result, contract.ContextDoc{
			Content: item.Content,
			Source:  cloneStringPtr(item.Source),
			Title:   cloneStringPtr(item.Title),
		})
	}
	return result
}

func clonePromptOverrides(overrides *contract.PromptOverrides) *contract.PromptOverrides {
	if overrides == nil {
		return nil
	}
	return &contract.PromptOverrides{
		Protocol:           cloneStringPtr(overrides.Protocol),
		OutputInstructions: cloneStringPtr(overrides.OutputInstructions),
	}
}
