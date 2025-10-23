package webaction

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jrzesz33/rez_agent/internal/models"
)

// ActionHandler defines the interface for web action handlers
type ActionHandler interface {
	// Execute performs the web action and returns the formatted notification message
	Execute(ctx context.Context, payload *models.WebActionPayload) (string, error)

	// GetActionType returns the action type this handler supports
	GetActionType() models.WebActionType
}

// HandlerRegistry manages action handlers
type HandlerRegistry struct {
	handlers map[models.WebActionType]ActionHandler
	logger   *slog.Logger
}

// NewHandlerRegistry creates a new handler registry
func NewHandlerRegistry(logger *slog.Logger) *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[models.WebActionType]ActionHandler),
		logger:   logger,
	}
}

// Register adds a handler to the registry
func (r *HandlerRegistry) Register(handler ActionHandler) error {
	actionType := handler.GetActionType()

	if _, exists := r.handlers[actionType]; exists {
		return fmt.Errorf("handler for action type %s already registered", actionType)
	}

	r.handlers[actionType] = handler
	r.logger.Info("registered action handler",
		slog.String("action_type", actionType.String()),
	)

	return nil
}

// GetHandler retrieves a handler for the given action type
func (r *HandlerRegistry) GetHandler(actionType models.WebActionType) (ActionHandler, error) {
	handler, exists := r.handlers[actionType]
	if !exists {
		return nil, fmt.Errorf("no handler registered for action type: %s", actionType)
	}

	return handler, nil
}

// ListHandlers returns all registered action types
func (r *HandlerRegistry) ListHandlers() []models.WebActionType {
	actionTypes := make([]models.WebActionType, 0, len(r.handlers))
	for actionType := range r.handlers {
		actionTypes = append(actionTypes, actionType)
	}
	return actionTypes
}
