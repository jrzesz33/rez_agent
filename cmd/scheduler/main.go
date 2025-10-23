package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/jrzesz33/rez_agent/internal/messaging"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/repository"
	appconfig "github.com/jrzesz33/rez_agent/pkg/config"
)

// SchedulerHandler handles EventBridge scheduled events
type SchedulerHandler struct {
	config     *appconfig.Config
	repository repository.MessageRepository
	publisher  messaging.SNSPublisher
	logger     *slog.Logger
}

// NewSchedulerHandler creates a new scheduler handler instance
func NewSchedulerHandler(
	cfg *appconfig.Config,
	repo repository.MessageRepository,
	pub messaging.SNSPublisher,
	logger *slog.Logger,
) *SchedulerHandler {
	return &SchedulerHandler{
		config:     cfg,
		repository: repo,
		publisher:  pub,
		logger:     logger,
	}
}

// HandleEvent processes the scheduled event
func (h *SchedulerHandler) HandleEvent(ctx context.Context, event interface{}) error {
	h.logger.InfoContext(ctx, "scheduler triggered",
		slog.String("stage", h.config.Stage.String()),
	)

	// Create a new "hello world" message
	message := models.NewMessage(
		"scheduler",
		h.config.Stage,
		models.MessageTypeScheduled,
		"Hello World! This is a scheduled message from rez_agent.",
	)

	h.logger.InfoContext(ctx, "created message",
		slog.String("message_id", message.ID),
		slog.String("type", message.MessageType.String()),
	)

	// Save message to DynamoDB
	err := h.repository.SaveMessage(ctx, message)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to save message",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to save message: %w", err)
	}

	h.logger.InfoContext(ctx, "saved message to DynamoDB",
		slog.String("message_id", message.ID),
	)

	// Mark message as queued
	message.MarkQueued()

	// Update message status in DynamoDB
	err = h.repository.UpdateStatus(ctx, message.ID, message.Status, "")
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to update message status",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to update message status: %w", err)
	}

	// Publish message to SNS
	err = h.publisher.PublishMessage(ctx, message)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to publish message",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("failed to publish message: %w", err)
	}

	h.logger.InfoContext(ctx, "successfully processed scheduled event",
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

	logger.Info("scheduler lambda starting",
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
	snsClient := sns.NewFromConfig(awsCfg)

	// Create repository and publisher
	repo := repository.NewDynamoDBRepository(dynamoClient, cfg.DynamoDBTableName)
	publisher := messaging.NewSNSClient(snsClient, cfg.SNSTopicArn, logger)

	// Create handler
	handler := NewSchedulerHandler(cfg, repo, publisher, logger)

	// Start Lambda handler
	lambda.Start(handler.HandleEvent)
}
