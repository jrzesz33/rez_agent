package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
	"github.com/jrzesz33/rez_agent/internal/mcp/tools"
)

// MCPServer implements the Model Context Protocol server
type MCPServer struct {
	jsonrpcServer *JSONRPCServer
	toolRegistry  *tools.Registry
	serverInfo    protocol.MCPServerInfo
	logger        *slog.Logger
	initialized   bool
}

// NewMCPServer creates a new MCP server
func NewMCPServer(name, version string, logger *slog.Logger) *MCPServer {
	server := &MCPServer{
		jsonrpcServer: NewJSONRPCServer(logger),
		toolRegistry:  tools.NewRegistry(logger),
		serverInfo: protocol.MCPServerInfo{
			Name:            name,
			Version:         version,
			ProtocolVersion: protocol.MCPVersion,
			Capabilities: protocol.MCPServerCapabilities{
				Tools: &protocol.MCPToolsCapability{
					ListChanged: false,
				},
				Logging: &protocol.MCPLoggingCapability{},
			},
			Instructions: "This is the rez_agent MCP server. It provides tools for push notifications, weather information, and golf course operations.",
		},
		logger:      logger,
		initialized: false,
	}

	// Register MCP protocol methods
	server.registerMethods()

	return server
}

// registerMethods registers all MCP protocol methods
func (s *MCPServer) registerMethods() {
	s.jsonrpcServer.RegisterMethod("initialize", s.handleInitialize)
	s.jsonrpcServer.RegisterMethod("tools/list", s.handleToolsList)
	s.jsonrpcServer.RegisterMethod("tools/call", s.handleToolsCall)
	s.jsonrpcServer.RegisterMethod("ping", s.handlePing)
}

// RegisterTool registers a tool with the server
func (s *MCPServer) RegisterTool(tool tools.Tool) error {
	return s.toolRegistry.Register(tool)
}

// HandleRequest processes an MCP request
func (s *MCPServer) HandleRequest(ctx context.Context, requestData []byte) ([]byte, error) {
	// Check if it's a batch request
	if IsBatchRequest(requestData) {
		return s.jsonrpcServer.HandleBatch(ctx, requestData)
	}

	// Single request
	return s.jsonrpcServer.HandleRequest(ctx, requestData)
}

// handleInitialize handles the initialize method
func (s *MCPServer) handleInitialize(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req protocol.InitializeRequest
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, protocol.NewJSONRPCError(protocol.ErrCodeInvalidParams,
				"Invalid initialize parameters", err.Error())
		}
	}

	s.logger.Info("initialize request received",
		slog.String("client_name", req.ClientInfo.Name),
		slog.String("client_version", req.ClientInfo.Version),
		slog.String("protocol_version", req.ProtocolVersion),
	)

	// Validate protocol version compatibility
	if req.ProtocolVersion != "" && req.ProtocolVersion != protocol.MCPVersion {
		s.logger.Warn("protocol version mismatch",
			slog.String("requested", req.ProtocolVersion),
			slog.String("supported", protocol.MCPVersion),
		)
		// Continue anyway - versions might be compatible
	}

	s.initialized = true

	result := protocol.InitializeResult{
		ProtocolVersion: protocol.MCPVersion,
		ServerInfo:      s.serverInfo,
		Capabilities:    s.serverInfo.Capabilities,
		Instructions:    s.serverInfo.Instructions,
	}

	return result, nil
}

// handleToolsList handles the tools/list method
func (s *MCPServer) handleToolsList(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if !s.initialized {
		return nil, protocol.NewJSONRPCError(protocol.ErrCodeInvalidRequest,
			"Server not initialized", "Call initialize first")
	}

	var req protocol.ToolsListRequest
	if len(params) > 0 {
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, protocol.NewJSONRPCError(protocol.ErrCodeInvalidParams,
				"Invalid tools/list parameters", err.Error())
		}
	}

	s.logger.Debug("tools/list request received")

	toolsList := s.toolRegistry.ListTools()

	result := protocol.ToolsListResult{
		Tools: toolsList,
	}

	s.logger.Info("returning tools list",
		slog.Int("tool_count", len(toolsList)),
	)

	return result, nil
}

// handleToolsCall handles the tools/call method
func (s *MCPServer) handleToolsCall(ctx context.Context, params json.RawMessage) (interface{}, error) {
	if !s.initialized {
		return nil, protocol.NewJSONRPCError(protocol.ErrCodeInvalidRequest,
			"Server not initialized", "Call initialize first")
	}

	var req protocol.ToolCallRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, protocol.NewJSONRPCError(protocol.ErrCodeInvalidParams,
			"Invalid tools/call parameters", err.Error())
	}

	s.logger.Info("tools/call request received",
		slog.String("tool_name", req.Name),
	)

	// Get the tool
	tool, err := s.toolRegistry.GetTool(req.Name)
	if err != nil {
		return nil, protocol.NewJSONRPCError(protocol.ErrCodeToolNotFound,
			fmt.Sprintf("Tool not found: %s", req.Name), nil)
	}

	// Validate input
	if err := tool.ValidateInput(req.Arguments); err != nil {
		return nil, protocol.NewJSONRPCError(protocol.ErrCodeInvalidParams,
			fmt.Sprintf("Invalid tool input: %v", err), nil)
	}

	// Execute the tool
	content, err := tool.Execute(ctx, req.Arguments)
	if err != nil {
		s.logger.Error("tool execution failed",
			slog.String("tool_name", req.Name),
			slog.String("error", err.Error()),
		)

		// Return error as content
		result := protocol.ToolCallResult{
			Content: []protocol.Content{
				protocol.NewErrorContent(err.Error()),
			},
			IsError: true,
		}
		return result, nil
	}

	s.logger.Info("tool executed successfully",
		slog.String("tool_name", req.Name),
	)

	result := protocol.ToolCallResult{
		Content: content,
		IsError: false,
	}

	return result, nil
}

// handlePing handles ping requests (keepalive)
func (s *MCPServer) handlePing(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return map[string]string{"status": "pong"}, nil
}

// GetServerInfo returns the server information
func (s *MCPServer) GetServerInfo() protocol.MCPServerInfo {
	return s.serverInfo
}

// IsInitialized returns whether the server has been initialized
func (s *MCPServer) IsInitialized() bool {
	return s.initialized
}
