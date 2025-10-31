package tools

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jrzesz33/rez_agent/internal/mcp/protocol"
	"github.com/jrzesz33/rez_agent/internal/notification"
)

// NotificationTool implements the send_push_notification MCP tool
type NotificationTool struct {
	ntfyClient *notification.NtfyClient
	logger     *slog.Logger
}

// NewNotificationTool creates a new notification tool
func NewNotificationTool(ntfyURL string, logger *slog.Logger) *NotificationTool {
	return &NotificationTool{
		ntfyClient: notification.NewNtfyClient(notification.NtfyClientConfig{
			BaseURL: ntfyURL,
			Logger:  logger,
		}),
		logger: logger,
	}
}

// GetDefinition returns the tool's MCP definition
func (t *NotificationTool) GetDefinition() protocol.Tool {
	return protocol.Tool{
		Name:        "send_push_notification",
		Description: "Send a push notification via ntfy.sh to the configured alert topic",
		InputSchema: protocol.InputSchema{
			Type: "object",
			Properties: map[string]protocol.Property{
				"title": {
					Type:        "string",
					Description: "Notification title (optional)",
				},
				"message": {
					Type:        "string",
					Description: "Notification message content",
				},
				"priority": {
					Type:        "string",
					Description: "Notification priority level",
					Enum:        []string{"low", "default", "high"},
					Default:     "default",
				},
			},
			Required: []string{"message"},
		},
	}
}

// ValidateInput validates the tool's input arguments
func (t *NotificationTool) ValidateInput(args map[string]interface{}) error {
	return ValidateInputAgainstSchema(args, t.GetDefinition().InputSchema)
}

// Execute runs the tool with the given arguments
func (t *NotificationTool) Execute(ctx context.Context, args map[string]interface{}) ([]protocol.Content, error) {
	title := GetStringArg(args, "title", "rez_agent Notification")
	message := GetStringArg(args, "message", "")
	priority := GetStringArg(args, "priority", "default")

	if message == "" {
		return nil, fmt.Errorf("message cannot be empty")
	}

	t.logger.Info("sending push notification",
		slog.String("title", title),
		slog.String("priority", priority),
	)

	if err := t.ntfyClient.SendWithTitle(ctx, title, message); err != nil {
		return nil, fmt.Errorf("failed to send notification: %w", err)
	}

	return []protocol.Content{
		protocol.NewTextContent(fmt.Sprintf("✅ Notification sent successfully: %s", title)),
	}, nil
}
