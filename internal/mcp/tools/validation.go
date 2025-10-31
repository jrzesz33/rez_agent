package tools

import (
	"fmt"
	"reflect"

	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
)

// ValidateInputAgainstSchema validates input arguments against a JSON schema
func ValidateInputAgainstSchema(args map[string]interface{}, schema protocol.InputSchema) error {
	// Check required fields
	for _, required := range schema.Required {
		if _, exists := args[required]; !exists {
			return fmt.Errorf("required field missing: %s", required)
		}
	}

	// Validate each property
	for key, value := range args {
		prop, exists := schema.Properties[key]
		if !exists {
			// Unknown property - allow for now, could be strict and reject
			continue
		}

		if err := validateValue(key, value, prop); err != nil {
			return err
		}
	}

	return nil
}

// validateValue validates a single value against a property schema
func validateValue(fieldName string, value interface{}, prop protocol.Property) error {
	// Check type
	actualType := getJSONType(value)
	// Allow "number" for "integer" type since JSON numbers are all float64
	if prop.Type != "" && actualType != prop.Type {
		if !(prop.Type == "integer" && actualType == "number") {
			return fmt.Errorf("field %s: expected type %s, got %s", fieldName, prop.Type, actualType)
		}
	}

	// Type-specific validation
	switch prop.Type {
	case "string":
		strValue, ok := value.(string)
		if !ok {
			return fmt.Errorf("field %s: expected string", fieldName)
		}

		// Enum validation
		if len(prop.Enum) > 0 {
			found := false
			for _, enumVal := range prop.Enum {
				if strValue == enumVal {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("field %s: value must be one of %v", fieldName, prop.Enum)
			}
		}

		// Format validation (basic)
		if prop.Format != "" {
			if err := validateFormat(fieldName, strValue, prop.Format); err != nil {
				return err
			}
		}

	case "integer", "number":
		numValue, ok := value.(float64) // JSON numbers are float64
		if !ok {
			return fmt.Errorf("field %s: expected number", fieldName)
		}

		// Minimum validation
		if prop.Minimum != nil {
			if int(numValue) < *prop.Minimum {
				return fmt.Errorf("field %s: must be >= %d", fieldName, *prop.Minimum)
			}
		}

		// Maximum validation
		if prop.Maximum != nil {
			if int(numValue) > *prop.Maximum {
				return fmt.Errorf("field %s: must be <= %d", fieldName, *prop.Maximum)
			}
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field %s: expected boolean", fieldName)
		}

	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return fmt.Errorf("field %s: expected object", fieldName)
		}

	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("field %s: expected array", fieldName)
		}
	}

	return nil
}

// getJSONType returns the JSON type name for a Go value
func getJSONType(value interface{}) string {
	if value == nil {
		return "null"
	}

	switch value.(type) {
	case string:
		return "string"
	case float64, int, int32, int64:
		return "number"
	case bool:
		return "boolean"
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	default:
		return reflect.TypeOf(value).String()
	}
}

// validateFormat validates string formats (basic implementation)
func validateFormat(fieldName, value, format string) error {
	switch format {
	case "date":
		// Basic date format validation (YYYY-MM-DD)
		if len(value) != 10 || value[4] != '-' || value[7] != '-' {
			return fmt.Errorf("field %s: invalid date format, expected YYYY-MM-DD", fieldName)
		}
	case "email":
		// Basic email validation
		if len(value) < 3 || !contains(value, "@") {
			return fmt.Errorf("field %s: invalid email format", fieldName)
		}
	case "uri", "url":
		// Basic URL validation
		if len(value) < 7 || (!startsWith(value, "http://") && !startsWith(value, "https://")) {
			return fmt.Errorf("field %s: invalid URL format", fieldName)
		}
	}

	return nil
}

// Helper functions
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}

// GetStringArg safely extracts a string argument
func GetStringArg(args map[string]interface{}, key string, defaultValue string) string {
	if val, exists := args[key]; exists {
		if strVal, ok := val.(string); ok {
			return strVal
		}
	}
	return defaultValue
}

// GetIntArg safely extracts an integer argument
func GetIntArg(args map[string]interface{}, key string, defaultValue int) int {
	if val, exists := args[key]; exists {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return defaultValue
}

// GetBoolArg safely extracts a boolean argument
func GetBoolArg(args map[string]interface{}, key string, defaultValue bool) bool {
	if val, exists := args[key]; exists {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}
	return defaultValue
}
