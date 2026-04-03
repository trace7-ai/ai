package contract

import "fmt"

func validateTypedStringSlice(items []string, field string) ([]string, error) {
	if len(items) == 0 {
		return []string{}, nil
	}
	result := make([]string, 0, len(items))
	for index, item := range items {
		if item == "" {
			return nil, fmt.Errorf("%s[%d] must be a non-empty string", field, index)
		}
		result = append(result, item)
	}
	return result, nil
}

func validateOptionalField(value *string, field string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if *value == "" {
		return nil, fmt.Errorf("%s must be a non-empty string", field)
	}
	return stringPtr(*value), nil
}
