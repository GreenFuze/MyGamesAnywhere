package plugins

import (
	"fmt"
)

// ValidateConfig validates config against the plugin manifest's config schema.
// Schema format supports primitive fields plus nested array/object shapes used by filesystem-backed sources.
func ValidateConfig(config map[string]any, schema map[string]any) error {
	if schema == nil {
		return nil
	}
	if config == nil {
		config = map[string]any{}
	}
	return validateObject(config, schema, "config")
}

func validateObject(config map[string]any, schema map[string]any, path string) error {
	for fieldName, fieldSchema := range schema {
		fs, ok := fieldSchema.(map[string]any)
		if !ok {
			continue
		}
		required, _ := fs["required"].(bool)
		val, present := config[fieldName]
		if !present {
			if required {
				return fmt.Errorf("%s: field %q is required", path, fieldName)
			}
			continue
		}
		if err := validateValue(val, fs, fmt.Sprintf("%s.%s", path, fieldName)); err != nil {
			return err
		}
	}
	return nil
}

func validateValue(val any, schema map[string]any, fieldPath string) error {
	typeStr, _ := schema["type"].(string)
	switch typeStr {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Errorf("%s must be a string", fieldPath)
		}
	case "number", "integer":
		switch val.(type) {
		case float64, int, int64:
		default:
			return fmt.Errorf("%s must be a number", fieldPath)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", fieldPath)
		}
	case "array":
		items, ok := toAnySlice(val)
		if !ok {
			return fmt.Errorf("%s must be an array", fieldPath)
		}
		itemSchema, _ := schema["items"].(map[string]any)
		for i, item := range items {
			if err := validateValue(item, itemSchema, fmt.Sprintf("%s[%d]", fieldPath, i)); err != nil {
				return err
			}
		}
	case "object":
		obj, ok := val.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", fieldPath)
		}
		properties, _ := schema["properties"].(map[string]any)
		if err := validateObject(obj, properties, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

func toAnySlice(val any) ([]any, bool) {
	switch typed := val.(type) {
	case []any:
		return typed, true
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out, true
	default:
		return nil, false
	}
}
