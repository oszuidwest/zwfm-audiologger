// Package utils provides modern Go 1.25 generic utilities for type safety
package utils

// CloneMap creates a deep copy of a map where values are pointers
func CloneMap[K comparable, V any](source map[K]*V) map[K]V {
	if source == nil {
		return nil
	}

	result := make(map[K]V, len(source))
	for k, v := range source {
		if v != nil {
			result[k] = *v
		}
	}
	return result
}
