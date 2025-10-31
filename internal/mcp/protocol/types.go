package protocol

import (
	"encoding/json"
	"fmt"
)

// MCPVersion is the MCP protocol version
const MCPVersion = "2025-03-26"

// JSONRPCVersion is the JSON-RPC version used by MCP
const JSONRPCVersion = "2.0"

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"` // string, number, or null
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error codes as per JSON-RPC 2.0 and MCP spec
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603

	// MCP-specific error codes
	ErrCodeToolNotFound  = -32001
	ErrCodeToolExecution = -32002
	ErrCodeAsyncTimeout  = -32003
	ErrCodeAuthFailure   = -32004
)

// NewJSONRPCError creates a new JSON-RPC error
func NewJSONRPCError(code int, message string, data interface{}) *JSONRPCError {
	err := &JSONRPCError{
		Code:    code,
		Message: message,
	}
	if data != nil {
		dataBytes, _ := json.Marshal(data)
		err.Data = dataBytes
	}
	return err
}

// Error implements the error interface
func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// MCPServerInfo represents MCP server information
type MCPServerInfo struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    MCPServerCapabilities  `json:"capabilities"`
	Instructions    string                 `json:"instructions,omitempty"`
}

// MCPServerCapabilities defines what the server supports
type MCPServerCapabilities struct {
	Tools      *MCPToolsCapability      `json:"tools,omitempty"`
	Resources  *MCPResourcesCapability  `json:"resources,omitempty"`
	Prompts    *MCPPromptsCapability    `json:"prompts,omitempty"`
	Logging    *MCPLoggingCapability    `json:"logging,omitempty"`
}

// MCPToolsCapability indicates tool support
type MCPToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPResourcesCapability indicates resource support
type MCPResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPPromptsCapability indicates prompt support
type MCPPromptsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPLoggingCapability indicates logging support
type MCPLoggingCapability struct{}

// MCPClientInfo represents MCP client information
type MCPClientInfo struct {
	Name            string                 `json:"name"`
	Version         string                 `json:"version"`
	ProtocolVersion string                 `json:"protocolVersion,omitempty"`
	Capabilities    MCPClientCapabilities  `json:"capabilities,omitempty"`
}

// MCPClientCapabilities defines what the client supports
type MCPClientCapabilities struct {
	Roots    *MCPRootsCapability    `json:"roots,omitempty"`
	Sampling *MCPSamplingCapability `json:"sampling,omitempty"`
}

// MCPRootsCapability indicates roots support
type MCPRootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// MCPSamplingCapability indicates sampling support
type MCPSamplingCapability struct{}

// Tool represents an MCP tool definition
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	InputSchema InputSchema `json:"inputSchema"`
}

// InputSchema represents a JSON Schema for tool input validation
type InputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]Property    `json:"properties,omitempty"`
	Required   []string               `json:"required,omitempty"`
	Additional map[string]interface{} `json:"-"` // For any additional schema properties
}

// Property represents a JSON Schema property
type Property struct {
	Type        string      `json:"type"`
	Description string      `json:"description,omitempty"`
	Enum        []string    `json:"enum,omitempty"`
	Format      string      `json:"format,omitempty"`
	Minimum     *int        `json:"minimum,omitempty"`
	Maximum     *int        `json:"maximum,omitempty"`
	Default     interface{} `json:"default,omitempty"`
}

// ToolsListRequest represents a request to list available tools
type ToolsListRequest struct {
	// No parameters needed
}

// ToolsListResult represents the result of listing tools
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallRequest represents a request to call a tool
type ToolCallRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// ToolCallResult represents the result of calling a tool
type ToolCallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content represents content in a tool result
type Content struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	Data     string      `json:"data,omitempty"`     // For base64 data
	MimeType string      `json:"mimeType,omitempty"` // For binary data
	Resource interface{} `json:"resource,omitempty"` // For resource content
}

// NewTextContent creates a new text content item
func NewTextContent(text string) Content {
	return Content{
		Type: "text",
		Text: text,
	}
}

// NewErrorContent creates a new error content item
func NewErrorContent(errorMsg string) Content {
	return Content{
		Type: "text",
		Text: fmt.Sprintf("Error: %s", errorMsg),
	}
}

// InitializeRequest represents the initialize request
type InitializeRequest struct {
	ProtocolVersion string        `json:"protocolVersion"`
	ClientInfo      MCPClientInfo `json:"clientInfo"`
	Capabilities    MCPClientCapabilities `json:"capabilities,omitempty"`
}

// InitializeResult represents the initialize result
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	ServerInfo      MCPServerInfo          `json:"serverInfo"`
	Capabilities    MCPServerCapabilities  `json:"capabilities"`
	Instructions    string                 `json:"instructions,omitempty"`
}

// Notification represents a JSON-RPC notification (no response expected)
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// LoggingLevel represents log levels
type LoggingLevel string

const (
	LogLevelDebug     LoggingLevel = "debug"
	LogLevelInfo      LoggingLevel = "info"
	LogLevelNotice    LoggingLevel = "notice"
	LogLevelWarning   LoggingLevel = "warning"
	LogLevelError     LoggingLevel = "error"
	LogLevelCritical  LoggingLevel = "critical"
	LogLevelAlert     LoggingLevel = "alert"
	LogLevelEmergency LoggingLevel = "emergency"
)

// LoggingMessageNotification represents a logging message notification
type LoggingMessageNotification struct {
	Level  LoggingLevel `json:"level"`
	Logger string       `json:"logger,omitempty"`
	Data   interface{}  `json:"data,omitempty"`
}
