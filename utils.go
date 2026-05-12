package main

import (
	"fmt"
	"time"
)

func extractOptionalString(raw interface{}) string {
	if raw == nil {
		return ""
	}

	if value, ok := raw.(string); ok {
		return value
	}

	return fmt.Sprintf("%v", raw)
}

func extractOptionalInt(raw interface{}) (*int, error) {
	if raw == nil {
		return nil, nil
	}

	switch v := raw.(type) {
	case float64:
		i := int(v)
		return &i, nil
	case int:
		return &v, nil
	case string:
		var i int
		_, err := fmt.Sscanf(v, "%d", &i)
		if err != nil {
			return nil, err
		}
		return &i, nil
	default:
		return nil, fmt.Errorf("cannot convert %v to int", raw)
	}
}

func extractOptionalTime(raw interface{}) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}

	str, ok := raw.(string)
	if !ok || str == "" {
		return nil, nil
	}

	t, err := time.Parse(time.RFC3339, str)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

func mustInt(raw interface{}) *int {
	val, err := extractOptionalInt(raw)
	if err != nil {
		return nil
	}
	return val
}