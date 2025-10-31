package tools

import (
	"testing"

	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
)

func TestValidateInputAgainstSchema_RequiredFields(t *testing.T) {
	schema := protocol.InputSchema{
		Type: "object",
		Properties: map[string]protocol.Property{
			"name": {
				Type:        "string",
				Description: "Name field",
			},
			"age": {
				Type:        "integer",
				Description: "Age field",
			},
		},
		Required: []string{"name"},
	}

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{
			name: "all required fields present",
			args: map[string]interface{}{
				"name": "John",
			},
			wantErr: false,
		},
		{
			name:    "missing required field",
			args:    map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "extra fields allowed",
			args: map[string]interface{}{
				"name":  "John",
				"extra": "field",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputAgainstSchema(tt.args, schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInputAgainstSchema() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInputAgainstSchema_StringType(t *testing.T) {
	schema := protocol.InputSchema{
		Type: "object",
		Properties: map[string]protocol.Property{
			"message": {
				Type:        "string",
				Description: "A message",
			},
		},
		Required: []string{"message"},
	}

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid string",
			args: map[string]interface{}{
				"message": "Hello, world!",
			},
			wantErr: false,
		},
		{
			name: "empty string",
			args: map[string]interface{}{
				"message": "",
			},
			wantErr: false,
		},
		{
			name: "invalid type - number",
			args: map[string]interface{}{
				"message": 123,
			},
			wantErr: true,
		},
		{
			name: "invalid type - boolean",
			args: map[string]interface{}{
				"message": true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputAgainstSchema(tt.args, schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInputAgainstSchema() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInputAgainstSchema_EnumValidation(t *testing.T) {
	schema := protocol.InputSchema{
		Type: "object",
		Properties: map[string]protocol.Property{
			"priority": {
				Type:        "string",
				Description: "Priority level",
				Enum:        []string{"low", "medium", "high"},
			},
		},
		Required: []string{"priority"},
	}

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid enum value",
			args: map[string]interface{}{
				"priority": "high",
			},
			wantErr: false,
		},
		{
			name: "invalid enum value",
			args: map[string]interface{}{
				"priority": "critical",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputAgainstSchema(tt.args, schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInputAgainstSchema() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInputAgainstSchema_IntegerType(t *testing.T) {
	schema := protocol.InputSchema{
		Type: "object",
		Properties: map[string]protocol.Property{
			"count": {
				Type:        "integer",
				Description: "A count",
				Minimum:     intPtr(1),
				Maximum:     intPtr(100),
			},
		},
		Required: []string{"count"},
	}

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid integer within range",
			args: map[string]interface{}{
				"count": float64(50), // JSON numbers are float64
			},
			wantErr: false,
		},
		{
			name: "integer at minimum",
			args: map[string]interface{}{
				"count": float64(1),
			},
			wantErr: false,
		},
		{
			name: "integer at maximum",
			args: map[string]interface{}{
				"count": float64(100),
			},
			wantErr: false,
		},
		{
			name: "integer below minimum",
			args: map[string]interface{}{
				"count": float64(0),
			},
			wantErr: true,
		},
		{
			name: "integer above maximum",
			args: map[string]interface{}{
				"count": float64(101),
			},
			wantErr: true,
		},
		{
			name: "invalid type - string",
			args: map[string]interface{}{
				"count": "50",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputAgainstSchema(tt.args, schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInputAgainstSchema() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInputAgainstSchema_BooleanType(t *testing.T) {
	schema := protocol.InputSchema{
		Type: "object",
		Properties: map[string]protocol.Property{
			"enabled": {
				Type:        "boolean",
				Description: "Is enabled",
			},
		},
		Required: []string{"enabled"},
	}

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid boolean true",
			args: map[string]interface{}{
				"enabled": true,
			},
			wantErr: false,
		},
		{
			name: "valid boolean false",
			args: map[string]interface{}{
				"enabled": false,
			},
			wantErr: false,
		},
		{
			name: "invalid type - string",
			args: map[string]interface{}{
				"enabled": "true",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputAgainstSchema(tt.args, schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInputAgainstSchema() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInputAgainstSchema_ObjectType(t *testing.T) {
	schema := protocol.InputSchema{
		Type: "object",
		Properties: map[string]protocol.Property{
			"metadata": {
				Type:        "object",
				Description: "Metadata object",
			},
		},
		Required: []string{"metadata"},
	}

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid object",
			args: map[string]interface{}{
				"metadata": map[string]interface{}{
					"key": "value",
				},
			},
			wantErr: false,
		},
		{
			name: "empty object",
			args: map[string]interface{}{
				"metadata": map[string]interface{}{},
			},
			wantErr: false,
		},
		{
			name: "invalid type - array",
			args: map[string]interface{}{
				"metadata": []interface{}{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputAgainstSchema(tt.args, schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInputAgainstSchema() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInputAgainstSchema_ArrayType(t *testing.T) {
	schema := protocol.InputSchema{
		Type: "object",
		Properties: map[string]protocol.Property{
			"items": {
				Type:        "array",
				Description: "Array of items",
			},
		},
		Required: []string{"items"},
	}

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid array",
			args: map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
			},
			wantErr: false,
		},
		{
			name: "empty array",
			args: map[string]interface{}{
				"items": []interface{}{},
			},
			wantErr: false,
		},
		{
			name: "invalid type - object",
			args: map[string]interface{}{
				"items": map[string]interface{}{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputAgainstSchema(tt.args, schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInputAgainstSchema() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFormat_Date(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:    "valid date",
			value:   "2025-10-31",
			wantErr: false,
		},
		{
			name:    "invalid date - wrong format",
			value:   "10/31/2025",
			wantErr: true,
		},
		{
			name:    "invalid date - too short",
			value:   "2025-10",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFormat("test_field", tt.value, "date")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFormat_Email(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:    "valid email",
			value:   "test@example.com",
			wantErr: false,
		},
		{
			name:    "invalid email - no @",
			value:   "testexample.com",
			wantErr: true,
		},
		{
			name:    "invalid email - too short",
			value:   "a@",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFormat("test_field", tt.value, "email")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFormat_URL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:    "valid http URL",
			value:   "http://example.com",
			wantErr: false,
		},
		{
			name:    "valid https URL",
			value:   "https://example.com",
			wantErr: false,
		},
		{
			name:    "invalid URL - no protocol",
			value:   "example.com",
			wantErr: true,
		},
		{
			name:    "invalid URL - wrong protocol",
			value:   "ftp://example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFormat("test_field", tt.value, "url")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetStringArg(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]interface{}
		key          string
		defaultValue string
		want         string
	}{
		{
			name: "existing string value",
			args: map[string]interface{}{
				"message": "Hello",
			},
			key:          "message",
			defaultValue: "default",
			want:         "Hello",
		},
		{
			name:         "missing key",
			args:         map[string]interface{}{},
			key:          "message",
			defaultValue: "default",
			want:         "default",
		},
		{
			name: "wrong type",
			args: map[string]interface{}{
				"message": 123,
			},
			key:          "message",
			defaultValue: "default",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetStringArg(tt.args, tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("GetStringArg() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetIntArg(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]interface{}
		key          string
		defaultValue int
		want         int
	}{
		{
			name: "existing float64 value",
			args: map[string]interface{}{
				"count": float64(42),
			},
			key:          "count",
			defaultValue: 0,
			want:         42,
		},
		{
			name: "existing int value",
			args: map[string]interface{}{
				"count": 42,
			},
			key:          "count",
			defaultValue: 0,
			want:         42,
		},
		{
			name:         "missing key",
			args:         map[string]interface{}{},
			key:          "count",
			defaultValue: 10,
			want:         10,
		},
		{
			name: "wrong type",
			args: map[string]interface{}{
				"count": "42",
			},
			key:          "count",
			defaultValue: 10,
			want:         10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetIntArg(tt.args, tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("GetIntArg() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBoolArg(t *testing.T) {
	tests := []struct {
		name         string
		args         map[string]interface{}
		key          string
		defaultValue bool
		want         bool
	}{
		{
			name: "existing true value",
			args: map[string]interface{}{
				"enabled": true,
			},
			key:          "enabled",
			defaultValue: false,
			want:         true,
		},
		{
			name: "existing false value",
			args: map[string]interface{}{
				"enabled": false,
			},
			key:          "enabled",
			defaultValue: true,
			want:         false,
		},
		{
			name:         "missing key",
			args:         map[string]interface{}{},
			key:          "enabled",
			defaultValue: true,
			want:         true,
		},
		{
			name: "wrong type",
			args: map[string]interface{}{
				"enabled": "true",
			},
			key:          "enabled",
			defaultValue: false,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBoolArg(tt.args, tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("GetBoolArg() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetJSONType(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{
			name:  "string type",
			value: "hello",
			want:  "string",
		},
		{
			name:  "float64 type",
			value: float64(123),
			want:  "number",
		},
		{
			name:  "int type",
			value: int(123),
			want:  "number",
		},
		{
			name:  "boolean type",
			value: true,
			want:  "boolean",
		},
		{
			name:  "object type",
			value: map[string]interface{}{},
			want:  "object",
		},
		{
			name:  "array type",
			value: []interface{}{},
			want:  "array",
		},
		{
			name:  "null type",
			value: nil,
			want:  "null",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getJSONType(tt.value)
			if got != tt.want {
				t.Errorf("getJSONType() = %v, want %v", got, tt.want)
			}
		})
	}
}
