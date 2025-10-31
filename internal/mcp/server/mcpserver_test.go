package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
)

// MockTool is a test implementation of the Tool interface
type MockTool struct {
	name        string
	description string
	executeFunc func(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error)
}

func (m *MockTool) GetDefinition() protocol.Tool {
	return protocol.Tool{
		Name:        m.name,
		Description: m.description,
		InputSchema: protocol.InputSchema{
			Type: "object",
			Properties: map[string]protocol.Property{
				"test_param": {
					Type:        "string",
					Description: "A test parameter",
				},
			},
			Required: []string{"test_param"},
		},
	}
}

func (m *MockTool) ValidateInput(args map[string]interface{}) error {
	if _, exists := args["test_param"]; !exists {
		return protocol.NewJSONRPCError(protocol.ErrCodeInvalidParams, "Missing test_param", nil)
	}
	return nil
}

func (m *MockTool) Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, args)
	}
	return []protocol.Content{
		protocol.NewTextContent("Mock tool executed successfully"),
	}, nil
}

func TestNewMCPServer(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := NewMCPServer("test-server", "1.0.0", logger)

	if server == nil {
		t.Fatal("NewMCPServer() returned nil")
	}
}

func TestMCPServer_RegisterTool(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := NewMCPServer("test-server", "1.0.0", logger)

	mockTool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
	}

	err := server.RegisterTool(mockTool)
	if err != nil {
		t.Errorf("RegisterTool() error = %v", err)
	}

	// Try to register the same tool again (should error)
	err = server.RegisterTool(mockTool)
	if err == nil {
		t.Error("RegisterTool() should error when registering duplicate tool")
	}
}

func TestMCPServer_Initialize(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := NewMCPServer("test-server", "1.0.0", logger)

	request := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2025-03-26",
			"capabilities": {},
			"clientInfo": {
				"name": "test-client",
				"version": "1.0.0"
			}
		}`),
	}

	requestData, _ := json.Marshal(request)
	responseData, err := server.HandleRequest(context.Background(), requestData)

	if err != nil {
		t.Fatalf("HandleRequest() error = %v", err)
	}

	var response protocol.JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("Initialize returned error: %v", response.Error)
	}

	if response.Result == nil {
		t.Fatal("Initialize result is nil")
	}

	// Verify result structure
	var result map[string]interface{}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["protocolVersion"] != "2025-03-26" {
		t.Errorf("protocolVersion = %v, want 2025-03-26", result["protocolVersion"])
	}

	serverInfo, ok := result["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatal("serverInfo is not a map")
	}

	if serverInfo["name"] != "test-server" {
		t.Errorf("serverInfo.name = %v, want test-server", serverInfo["name"])
	}

	if serverInfo["version"] != "1.0.0" {
		t.Errorf("serverInfo.version = %v, want 1.0.0", serverInfo["version"])
	}
}

func TestMCPServer_Ping(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := NewMCPServer("test-server", "1.0.0", logger)

	request := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "ping",
	}

	requestData, _ := json.Marshal(request)
	responseData, err := server.HandleRequest(context.Background(), requestData)

	if err != nil {
		t.Fatalf("HandleRequest() error = %v", err)
	}

	var response protocol.JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("Ping returned error: %v", response.Error)
	}

	// Ping should return status: pong
	var result map[string]interface{}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["status"] != "pong" {
		t.Errorf("Ping status = %v, want pong", result["status"])
	}
}

func TestMCPServer_ToolsList(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := NewMCPServer("test-server", "1.0.0", logger)

	// Initialize the server first
	initRequest := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "0",
		Method:  "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2025-03-26",
			"clientInfo": {"name": "test", "version": "1.0.0"}
		}`),
	}
	initData, _ := json.Marshal(initRequest)
	_, err := server.HandleRequest(context.Background(), initData)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Register a mock tool
	mockTool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	server.RegisterTool(mockTool)

	request := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "tools/list",
	}

	requestData, _ := json.Marshal(request)
	responseData, err := server.HandleRequest(context.Background(), requestData)

	if err != nil {
		t.Fatalf("HandleRequest() error = %v", err)
	}

	var response protocol.JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("tools/list returned error: %v", response.Error)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("tools is not an array")
	}

	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}

	tool := tools[0].(map[string]interface{})
	if tool["name"] != "test_tool" {
		t.Errorf("Tool name = %v, want test_tool", tool["name"])
	}

	if tool["description"] != "A test tool" {
		t.Errorf("Tool description = %v, want 'A test tool'", tool["description"])
	}
}

func TestMCPServer_ToolsCall_Success(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := NewMCPServer("test-server", "1.0.0", logger)

	// Initialize the server first
	initRequest := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "0",
		Method:  "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2025-03-26",
			"clientInfo": {"name": "test", "version": "1.0.0"}
		}`),
	}
	initData, _ := json.Marshal(initRequest)
	_, err := server.HandleRequest(context.Background(), initData)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Register a mock tool
	mockTool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
		executeFunc: func(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
			param := args["test_param"].(string)
			return []protocol.Content{
				protocol.NewTextContent("Received: " + param),
			}, nil
		},
	}
	server.RegisterTool(mockTool)

	request := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "test_tool",
			"arguments": {
				"test_param": "hello"
			}
		}`),
	}

	requestData, _ := json.Marshal(request)
	responseData, err := server.HandleRequest(context.Background(), requestData)

	if err != nil {
		t.Fatalf("HandleRequest() error = %v", err)
	}

	var response protocol.JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("tools/call returned error: %v", response.Error)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	content, ok := result["content"].([]interface{})
	if !ok {
		t.Fatal("content is not an array")
	}

	if len(content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(content))
	}

	contentItem := content[0].(map[string]interface{})
	if contentItem["type"] != "text" {
		t.Errorf("Content type = %v, want text", contentItem["type"])
	}

	if contentItem["text"] != "Received: hello" {
		t.Errorf("Content text = %v, want 'Received: hello'", contentItem["text"])
	}
}

func TestMCPServer_ToolsCall_ToolNotFound(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := NewMCPServer("test-server", "1.0.0", logger)

	// Initialize the server first
	initRequest := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "0",
		Method:  "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2025-03-26",
			"clientInfo": {"name": "test", "version": "1.0.0"}
		}`),
	}
	initData, _ := json.Marshal(initRequest)
	_, err := server.HandleRequest(context.Background(), initData)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	request := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nonexistent_tool",
			"arguments": {}
		}`),
	}

	requestData, _ := json.Marshal(request)
	responseData, err := server.HandleRequest(context.Background(), requestData)

	if err != nil {
		t.Fatalf("HandleRequest() error = %v", err)
	}

	var response protocol.JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error == nil {
		t.Fatal("Expected error for nonexistent tool, got nil")
	}

	if response.Error.Code != protocol.ErrCodeToolNotFound {
		t.Errorf("Error code = %d, want %d", response.Error.Code, protocol.ErrCodeToolNotFound)
	}
}

func TestMCPServer_ToolsCall_ValidationError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := NewMCPServer("test-server", "1.0.0", logger)

	// Initialize the server first
	initRequest := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "0",
		Method:  "initialize",
		Params: json.RawMessage(`{
			"protocolVersion": "2025-03-26",
			"clientInfo": {"name": "test", "version": "1.0.0"}
		}`),
	}
	initData, _ := json.Marshal(initRequest)
	_, err := server.HandleRequest(context.Background(), initData)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Register a mock tool
	mockTool := &MockTool{
		name:        "test_tool",
		description: "A test tool",
	}
	server.RegisterTool(mockTool)

	// Call without required parameter
	request := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "test_tool",
			"arguments": {}
		}`),
	}

	requestData, _ := json.Marshal(request)
	responseData, err := server.HandleRequest(context.Background(), requestData)

	if err != nil {
		t.Fatalf("HandleRequest() error = %v", err)
	}

	var response protocol.JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error == nil {
		t.Fatal("Expected validation error, got nil")
	}
}

func TestMCPServer_InvalidMethod(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	server := NewMCPServer("test-server", "1.0.0", logger)

	request := protocol.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "invalid/method",
	}

	requestData, _ := json.Marshal(request)
	responseData, err := server.HandleRequest(context.Background(), requestData)

	if err != nil {
		t.Fatalf("HandleRequest() error = %v", err)
	}

	var response protocol.JSONRPCResponse
	if err := json.Unmarshal(responseData, &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error == nil {
		t.Fatal("Expected method not found error, got nil")
	}

	if response.Error.Code != protocol.ErrCodeMethodNotFound {
		t.Errorf("Error code = %d, want %d", response.Error.Code, protocol.ErrCodeMethodNotFound)
	}
}
