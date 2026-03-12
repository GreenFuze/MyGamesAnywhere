package plugins

import (
	"fmt"
)

// ValidateConfig validates config against the plugin manifest's config schema.
// Schema format: map of field name -> { "type": "string"|"number"|"boolean", "required": true, "default": ... }.
// Fail-fast: returns an error describing the first validation failure.
func ValidateConfig(config map[string]any, schema map[string]any) error {
	if schema == nil {
		return nil
	}
	if config == nil {
		config = map[string]any{}
	}
	for fieldName, fieldSchema := range schema {
		fs, ok := fieldSchema.(map[string]any)
		if !ok {
			continue
		}
		required, _ := fs["required"].(bool)
		val, present := config[fieldName]
		if !present {
			if required {
				return fmt.Errorf("config: field %q is required", fieldName)
			}
			continue
		}
		typeStr, _ := fs["type"].(string)
		switch typeStr {
		case "string":
			if _, ok := val.(string); !ok {
				return fmt.Errorf("config: field %q must be a string", fieldName)
			}
		case "number":
			switch val.(type) {
			case float64, int, int64:
				// JSON numbers decode as float64
			default:
				return fmt.Errorf("config: field %q must be a number", fieldName)
			}
		case "boolean":
			if _, ok := val.(bool); !ok {
				return fmt.Errorf("config: field %q must be a boolean", fieldName)
			}
		}
	}
	return nil
}
