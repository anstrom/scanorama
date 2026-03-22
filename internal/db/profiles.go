// Package db provides scan profile database operations for scanorama.
package db

// ProfileFilters represents filters for listing profiles.
type ProfileFilters struct {
	ScanType string
}

// parsePostgreSQLArray converts a PostgreSQL array interface{} to []string.
func parsePostgreSQLArray(arrayInterface interface{}) []string {
	if arrayInterface == nil {
		return nil
	}

	arr, ok := arrayInterface.([]interface{})
	if !ok {
		return nil
	}

	result := make([]string, len(arr))
	for i, v := range arr {
		if s, ok := v.(string); ok {
			result[i] = s
		}
	}
	return result
}
