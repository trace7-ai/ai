package cli

import (
	"fmt"
	"strconv"
)

func assignInt(target *int, raw string, label string) error {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("%s must be an integer", label)
	}
	*target = value
	return nil
}
