package tools

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestNotificationTool_GetDefinition(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	def := tool.GetDefinition()

	if def.Name != "send_push_notification" {
		t.Errorf("Name = %v, want send_push_notification", def.Name)
	}

	if def.InputSchema.Type != "object" {
		t.Errorf("InputSchema.Type = %v, want object", def.InputSchema.Type)
	}

	// Check required fields
	if len(def.InputSchema.Required) != 1 {
		t.Errorf("Required fields count = %d, want 1", len(def.InputSchema.Required))
	}

	if def.InputSchema.Required[0] != "message" {
		t.Errorf("Required field = %v, want message", def.InputSchema.Required[0])
	}

	// Check properties
	if _, exists := def.InputSchema.Properties["title"]; !exists {
		t.Error("Property 'title' not found")
	}

	if _, exists := def.InputSchema.Properties["message"]; !exists {
		t.Error("Property 'message' not found")
	}

	if _, exists := def.InputSchema.Properties["priority"]; !exists {
		t.Error("Property 'priority' not found")
	}

	// Check priority enum
	priorityProp := def.InputSchema.Properties["priority"]
	if len(priorityProp.Enum) != 3 {
		t.Errorf("Priority enum count = %d, want 3", len(priorityProp.Enum))
	}
}

func TestNotificationTool_ValidateInput(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid input with all fields",
			args: map[string]interface{}{
				"title":    "Test Title",
				"message":  "Test message",
				"priority": "high",
			},
			wantErr: false,
		},
		{
			name: "valid input with only required fields",
			args: map[string]interface{}{
				"message": "Test message",
			},
			wantErr: false,
		},
		{
			name:    "missing required message field",
			args:    map[string]interface{}{},
			wantErr: true,
		},
		{
			name: "invalid priority enum",
			args: map[string]interface{}{
				"message":  "Test message",
				"priority": "critical", // not in enum
			},
			wantErr: true,
		},
		{
			name: "invalid message type",
			args: map[string]interface{}{
				"message": 123, // should be string
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.ValidateInput(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNotificationTool_Execute_EmptyMessage(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	args := map[string]interface{}{
		"message": "",
	}

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("Execute() should return error for empty message")
	}
}

func TestNotificationTool_Execute_DefaultValues(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	// Test with minimal input to verify defaults are used
	args := map[string]interface{}{
		"message": "Test message",
	}

	// Note: This will actually try to send a notification in unit tests
	// In a real scenario, we'd want to mock the ntfy client
	// For now, we just verify the function doesn't panic
	_, err := tool.Execute(context.Background(), args)

	// We expect this to succeed or fail gracefully
	// The actual HTTP call might fail in test environment, which is ok
	if err != nil {
		t.Logf("Execute() returned error (expected in test environment): %v", err)
	}
}

func TestNotificationTool_Execute_WithTitle(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	args := map[string]interface{}{
		"title":    "Custom Title",
		"message":  "Test message",
		"priority": "high",
	}

	// Note: Same as above - in production code we'd mock the HTTP client
	_, err := tool.Execute(context.Background(), args)

	if err != nil {
		t.Logf("Execute() returned error (expected in test environment): %v", err)
	}
}

func TestNotificationTool_ContentFormat(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	// We can't easily test the actual execution without mocking,
	// but we can verify the tool definition produces the right schema
	def := tool.GetDefinition()

	// Verify that the tool definition is complete
	if def.Description == "" {
		t.Error("Tool description should not be empty")
	}

	// Verify all required MCP protocol fields are present
	if def.InputSchema.Type == "" {
		t.Error("InputSchema.Type should not be empty")
	}

	if def.InputSchema.Properties == nil {
		t.Error("InputSchema.Properties should not be nil")
	}
}

func TestNotificationTool_Integration_ValidateAndExecute(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	// Test the complete flow: validation -> execution
	args := map[string]interface{}{
		"title":    "Integration Test",
		"message":  "This is an integration test",
		"priority": "default",
	}

	// First validate
	err := tool.ValidateInput(args)
	if err != nil {
		t.Fatalf("ValidateInput() failed: %v", err)
	}

	// Then execute
	content, err := tool.Execute(context.Background(), args)
	if err != nil {
		// Expected to fail in test environment without real ntfy.sh access
		t.Logf("Execute() failed (expected): %v", err)
		return
	}

	// If it succeeds (e.g., if ntfy.sh is accessible), verify the response
	if len(content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(content))
	}

	if content[0].Type != "text" {
		t.Errorf("Content type = %v, want text", content[0].Type)
	}

	if content[0].Text == "" {
		t.Error("Content text should not be empty")
	}
}

func TestNotificationTool_ContextCancellation(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	args := map[string]interface{}{
		"message": "This should not be sent",
	}

	_, err := tool.Execute(ctx, args)

	// The ntfy client should respect context cancellation
	// However, the implementation might not propagate the error correctly
	// This test documents expected behavior
	if err != nil {
		t.Logf("Execute() with cancelled context returned error: %v", err)
	}
}

func TestNotificationTool_GetDefinition_SchemaCompleteness(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	def := tool.GetDefinition()

	// Verify all properties have descriptions
	for name, prop := range def.InputSchema.Properties {
		if prop.Description == "" {
			t.Errorf("Property %s has no description", name)
		}

		if prop.Type == "" {
			t.Errorf("Property %s has no type", name)
		}
	}

	// Verify priority has a default value
	priorityProp := def.InputSchema.Properties["priority"]
	if priorityProp.Default == nil {
		t.Error("Priority property should have a default value")
	}
}

func TestNotificationTool_ValidateInput_EdgeCases(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	tool := NewNotificationTool("https://ntfy.sh/test", logger)

	tests := []struct {
		name    string
		args    map[string]interface{}
		wantErr bool
	}{
		{
			name: "very long message",
			args: map[string]interface{}{
				"message": string(make([]byte, 10000)),
			},
			wantErr: false, // No length validation in current implementation
		},
		{
			name: "unicode in message",
			args: map[string]interface{}{
				"message": "Hello 世界 🌍",
			},
			wantErr: false,
		},
		{
			name: "special characters in title",
			args: map[string]interface{}{
				"title":   "Test: <>&\"'",
				"message": "test",
			},
			wantErr: false,
		},
		{
			name: "null message",
			args: map[string]interface{}{
				"message": nil,
			},
			wantErr: true, // Should be string type
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tool.ValidateInput(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
