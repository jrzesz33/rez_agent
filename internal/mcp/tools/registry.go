package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
)

// Tool represents an MCP tool that can be executed
type Tool interface {
	// GetDefinition returns the tool's MCP definition
	GetDefinition() protocol.Tool

	// ValidateInput validates the tool's input arguments
	ValidateInput(args map[string]interface{}) error

	// Execute runs the tool with the given arguments
	Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error)
}

// Registry manages available MCP tools
type Registry struct {
	tools  map[string]Tool
	logger *slog.Logger
}

// NewRegistry creates a new tool registry
func NewRegistry(logger *slog.Logger) *Registry {
	return &Registry{
		tools:  make(map[string]Tool),
		logger: logger,
	}
}

// Register adds a tool to the registry
func (r *Registry) Register(tool Tool) error {
	definition := tool.GetDefinition()

	if _, exists := r.tools[definition.Name]; exists {
		return fmt.Errorf("tool already registered: %s", definition.Name)
	}

	r.tools[definition.Name] = tool
	r.logger.Info("registered MCP tool",
		slog.String("tool_name", definition.Name),
		slog.String("description", definition.Description),
	)

	return nil
}

// GetTool retrieves a tool by name
func (r *Registry) GetTool(name string) (Tool, error) {
	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	return tool, nil
}

// ListTools returns all registered tool definitions
func (r *Registry) ListTools() []protocol.Tool {
	tools := make([]protocol.Tool, 0, len(r.tools))

	for _, tool := range r.tools {
		tools = append(tools, tool.GetDefinition())
	}

	return tools
}

// Count returns the number of registered tools
func (r *Registry) Count() int {
	return len(r.tools)
}
