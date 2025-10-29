package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/jrzesz33/rez_agent/internal/messaging"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/notification"
	"github.com/jrzesz33/rez_agent/internal/repository"
	appconfig "github.com/jrzesz33/rez_agent/pkg/config"
)

// ProcessorHandler handles SQS messages and sends notifications
type ProcessorHandler struct {
	config           *appconfig.Config
	repository       repository.MessageRepository
	notificationClient notification.Client
	batchProcessor   *messaging.SQSBatchProcessor
	logger           *slog.Logger
}

// NewProcessorHandler creates a new processor handler instance
func NewProcessorHandler(
	cfg *appconfig.Config,
	repo repository.MessageRepository,
	notifClient notification.Client,
	logger *slog.Logger,
) *ProcessorHandler {
	return &ProcessorHandler{
		config:             cfg,
		repository:         repo,
		notificationClient: notifClient,
		batchProcessor:     messaging.NewSQSBatchProcessor(logger),
		logger:             logger,
	}
}

// HandleEvent processes SQS events
func (h *ProcessorHandler) HandleEvent(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	h.logger.InfoContext(ctx, "processing SQS batch",
		slog.Int("record_count", len(event.Records)),
		slog.String("stage", h.config.Stage.String()),
	)

	// Process batch using the batch processor
	response, err := h.batchProcessor.ProcessBatch(ctx, event, h.processMessage)
	if err != nil {
		h.logger.ErrorContext(ctx, "batch processing encountered errors",
			slog.String("error", err.Error()),
			slog.Int("failure_count", len(response.BatchItemFailures)),
		)
	}

	h.logger.InfoContext(ctx, "batch processing completed",
		slog.Int("total_records", len(event.Records)),
		slog.Int("failed_records", len(response.BatchItemFailures)),
	)

	return response, nil
}

// processMessage processes a single message
func (h *ProcessorHandler) processMessage(ctx context.Context, message *models.Message) error {
	h.logger.DebugContext(ctx, "processing message",
		slog.String("message_id", message.ID),
		slog.String("type", message.MessageType.String()),
		slog.String("current_status", message.Status.String()),
	)

	// Mark message as processing
	message.MarkProcessing()
	err := h.repository.UpdateStatus(ctx, message.ID, message.Status, "")
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to update status to processing",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
		// Continue processing even if status update fails
	}

	// Send notification to ntfy.sh
	notificationTitle := fmt.Sprintf("Rez Agent - %s", h.config.Stage.String())
	err = h.notificationClient.(*notification.NtfyClient).SendWithTitle(ctx, notificationTitle, message.Payload)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to send notification",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)

		// Mark message as failed
		message.MarkFailed(err.Error())
		message.IncrementRetry()

		updateErr := h.repository.UpdateStatus(ctx, message.ID, message.Status, message.ErrorMessage)
		if updateErr != nil {
			h.logger.ErrorContext(ctx, "failed to update status to failed",
				slog.String("message_id", message.ID),
				slog.String("error", updateErr.Error()),
			)
		}

		return fmt.Errorf("failed to send notification: %w", err)
	}

	h.logger.DebugContext(ctx, "notification sent successfully",
		slog.String("message_id", message.ID),
	)

	// Mark message as completed
	message.MarkCompleted()
	err = h.repository.UpdateStatus(ctx, message.ID, message.Status, "")
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to update status to completed",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
		// Don't return error as the main processing succeeded
	}

	h.logger.DebugContext(ctx, "message processed successfully",
		slog.String("message_id", message.ID),
		slog.String("status", message.Status.String()),
	)

	return nil
}

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Load configuration
	cfg := appconfig.MustLoad()

	logger.Info("processor lambda starting",
		slog.String("stage", cfg.Stage.String()),
		slog.String("region", cfg.AWSRegion),
	)

	// Initialize AWS SDK
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("failed to load AWS config", slog.String("error", err.Error()))
		panic(fmt.Sprintf("failed to load AWS config: %v", err))
	}

	// Create AWS clients
	dynamoClient := dynamodb.NewFromConfig(awsCfg)

	// Create repository
	repo := repository.NewDynamoDBRepository(dynamoClient, cfg.DynamoDBTableName)

	// Create notification client
	notifClient := notification.NewNtfyClient(notification.NtfyClientConfig{
		BaseURL:    cfg.NtfyURL,
		Timeout:    30 * time.Second,
		MaxRetries: 3,
		Logger:     logger,
	})

	// Create handler
	handler := NewProcessorHandler(cfg, repo, notifClient, logger)

	// Start Lambda handler
	lambda.Start(handler.HandleEvent)
}
