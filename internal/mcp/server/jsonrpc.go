package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
)

// MethodHandler is a function that handles a JSON-RPC method call
type MethodHandler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// JSONRPCServer handles JSON-RPC 2.0 protocol
type JSONRPCServer struct {
	methods map[string]MethodHandler
	logger  *slog.Logger
}

// NewJSONRPCServer creates a new JSON-RPC server
func NewJSONRPCServer(logger *slog.Logger) *JSONRPCServer {
	return &JSONRPCServer{
		methods: make(map[string]MethodHandler),
		logger:  logger,
	}
}

// RegisterMethod registers a method handler
func (s *JSONRPCServer) RegisterMethod(method string, handler MethodHandler) {
	s.methods[method] = handler
	s.logger.Debug("registered JSON-RPC method",
		slog.String("method", method),
	)
}

// HandleRequest processes a JSON-RPC request and returns a response
func (s *JSONRPCServer) HandleRequest(ctx context.Context, requestData []byte) ([]byte, error) {
	// Parse the request
	var req protocol.JSONRPCRequest
	if err := json.Unmarshal(requestData, &req); err != nil {
		s.logger.Error("failed to parse JSON-RPC request",
			slog.String("error", err.Error()),
		)
		return s.errorResponse(nil, protocol.ErrCodeParseError, "Parse error", nil)
	}

	// Validate JSON-RPC version
	if req.JSONRPC != protocol.JSONRPCVersion {
		return s.errorResponse(req.ID, protocol.ErrCodeInvalidRequest,
			fmt.Sprintf("Invalid JSON-RPC version, expected %s", protocol.JSONRPCVersion), nil)
	}

	// Validate method exists
	if req.Method == "" {
		return s.errorResponse(req.ID, protocol.ErrCodeInvalidRequest, "Method is required", nil)
	}

	// Find handler
	handler, exists := s.methods[req.Method]
	if !exists {
		s.logger.Warn("method not found",
			slog.String("method", req.Method),
		)
		return s.errorResponse(req.ID, protocol.ErrCodeMethodNotFound,
			fmt.Sprintf("Method not found: %s", req.Method), nil)
	}

	// Execute handler
	s.logger.Debug("executing JSON-RPC method",
		slog.String("method", req.Method),
		slog.Any("id", req.ID),
	)

	result, err := handler(ctx, req.Params)
	if err != nil {
		s.logger.Error("method execution failed",
			slog.String("method", req.Method),
			slog.String("error", err.Error()),
		)

		// Check if it's a JSON-RPC error
		if rpcErr, ok := err.(*protocol.JSONRPCError); ok {
			return s.errorResponse(req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
		}

		// Generic internal error
		return s.errorResponse(req.ID, protocol.ErrCodeInternalError, err.Error(), nil)
	}

	// Success response
	return s.successResponse(req.ID, result)
}

// successResponse creates a successful JSON-RPC response
func (s *JSONRPCServer) successResponse(id interface{}, result interface{}) ([]byte, error) {
	resultBytes, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("failed to marshal result",
			slog.String("error", err.Error()),
		)
		return s.errorResponse(id, protocol.ErrCodeInternalError, "Failed to marshal result", nil)
	}

	resp := protocol.JSONRPCResponse{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      id,
		Result:  resultBytes,
	}

	return json.Marshal(resp)
}

// errorResponse creates an error JSON-RPC response
func (s *JSONRPCServer) errorResponse(id interface{}, code int, message string, data interface{}) ([]byte, error) {
	resp := protocol.JSONRPCResponse{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      id,
		Error:   protocol.NewJSONRPCError(code, message, data),
	}

	return json.Marshal(resp)
}

// HandleBatch processes a batch of JSON-RPC requests
func (s *JSONRPCServer) HandleBatch(ctx context.Context, requestData []byte) ([]byte, error) {
	// Try to parse as array
	var requests []json.RawMessage
	if err := json.Unmarshal(requestData, &requests); err != nil {
		// Not a batch request
		return nil, fmt.Errorf("invalid batch request: %w", err)
	}

	if len(requests) == 0 {
		return s.errorResponse(nil, protocol.ErrCodeInvalidRequest, "Empty batch", nil)
	}

	// Process each request
	responses := make([]json.RawMessage, 0, len(requests))
	for _, reqData := range requests {
		respData, err := s.HandleRequest(ctx, reqData)
		if err != nil {
			s.logger.Error("failed to process batch request",
				slog.String("error", err.Error()),
			)
			continue
		}
		responses = append(responses, respData)
	}

	// Return batch response
	return json.Marshal(responses)
}

// IsBatchRequest checks if the request data represents a batch request
func IsBatchRequest(data []byte) bool {
	// Quick check: does it start with '['?
	for _, b := range data {
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			continue
		}
		return b == '['
	}
	return false
}
