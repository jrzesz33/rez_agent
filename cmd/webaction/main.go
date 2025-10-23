package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/jrzesz33/rez_agent/internal/httpclient"
	"github.com/jrzesz33/rez_agent/internal/messaging"
	"github.com/jrzesz33/rez_agent/internal/models"
	"github.com/jrzesz33/rez_agent/internal/repository"
	"github.com/jrzesz33/rez_agent/internal/secrets"
	"github.com/jrzesz33/rez_agent/internal/webaction"
	"github.com/jrzesz33/rez_agent/pkg/config"
)

type Handler struct {
	messageRepo      repository.MessageRepository
	resultRepo       repository.WebActionResultRepository
	snsPublisher     messaging.SNSPublisher
	handlerRegistry  *webaction.HandlerRegistry
	sqsProcessor     *messaging.SQSBatchProcessor
	logger           *slog.Logger
}

func main() {
	// Initialize structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", slog.String("error", err.Error()))
		panic(err)
	}

	// Initialize AWS SDK config
	awsCfg, err := awsConfig.LoadDefaultConfig(context.Background(),
		awsConfig.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("failed to load AWS SDK config", slog.String("error", err.Error()))
		panic(err)
	}

	// Initialize AWS clients
	dynamoClient := dynamodb.NewFromConfig(awsCfg)
	snsClient := sns.NewFromConfig(awsCfg)

	// Initialize repositories
	messageRepo := repository.NewDynamoDBRepository(dynamoClient, cfg.DynamoDBTableName)
	resultRepo := repository.NewDynamoDBWebActionRepository(dynamoClient, cfg.WebActionResultsTableName)

	// Initialize SNS publisher
	snsPublisher := messaging.NewSNSClient(snsClient, cfg.SNSTopicArn, logger)

	// Initialize SQS processor
	sqsProcessor := messaging.NewSQSBatchProcessor(logger)

	// Initialize HTTP client and secrets manager
	httpClient := httpclient.NewClient(logger)
	secretsManager := secrets.NewManager(awsCfg, logger)
	oauthClient := httpclient.NewOAuthClient(httpClient, secretsManager, logger)

	// Initialize action handler registry
	handlerRegistry := webaction.NewHandlerRegistry(logger)

	// Register handlers
	weatherHandler := webaction.NewWeatherHandler(httpClient, logger)
	if err := handlerRegistry.Register(weatherHandler); err != nil {
		logger.Error("failed to register weather handler", slog.String("error", err.Error()))
		panic(err)
	}

	golfHandler := webaction.NewGolfHandler(httpClient, oauthClient, secretsManager, logger)
	if err := handlerRegistry.Register(golfHandler); err != nil {
		logger.Error("failed to register golf handler", slog.String("error", err.Error()))
		panic(err)
	}

	logger.Info("web action processor initialized",
		slog.Int("registered_handlers", len(handlerRegistry.ListHandlers())),
	)

	// Create handler
	handler := &Handler{
		messageRepo:     messageRepo,
		resultRepo:      resultRepo,
		snsPublisher:    snsPublisher,
		handlerRegistry: handlerRegistry,
		sqsProcessor:    sqsProcessor,
		logger:          logger,
	}

	// Start Lambda
	lambda.Start(handler.HandleSQSEvent)
}

// HandleSQSEvent processes SQS messages containing web action requests
func (h *Handler) HandleSQSEvent(ctx context.Context, event events.SQSEvent) (events.SQSEventResponse, error) {
	h.logger.Info("received SQS event",
		slog.Int("record_count", len(event.Records)),
	)

	// Use SQS batch processor to handle messages with proper retry logic
	return h.sqsProcessor.ProcessBatch(ctx, event, h.processMessage)
}

// processMessage processes a single message
func (h *Handler) processMessage(ctx context.Context, message *models.Message) error {
	startTime := time.Now()

	h.logger.Info("processing web action message",
		slog.String("message_id", message.ID),
	)

	// Validate message type
	if message.MessageType != models.MessageTypeWebAction {
		return fmt.Errorf("invalid message type: expected %s, got %s", models.MessageTypeWebAction, message.MessageType)
	}

	// Update message status to processing
	message.MarkProcessing()
	if err := h.messageRepo.SaveMessage(ctx, message); err != nil {
		h.logger.Warn("failed to update message status to processing", slog.String("error", err.Error()))
	}

	// Parse web action payload
	payload, err := models.ParseWebActionPayload(message.Payload)
	if err != nil {
		h.markMessageFailed(ctx, message, fmt.Sprintf("Invalid payload: %v", err))
		return fmt.Errorf("failed to parse web action payload: %w", err)
	}

	// SECURITY: Log redacted payload
	redactedPayload := payload.RedactSensitiveData()
	h.logger.Info("parsed web action payload",
		slog.String("action", redactedPayload.Action.String()),
		slog.String("url", redactedPayload.URL),
	)

	// Create web action result record
	result := models.NewWebActionResult(message.ID, payload.Action, payload.URL, message.Stage)
	if err := h.resultRepo.SaveResult(ctx, result); err != nil {
		h.logger.Warn("failed to save initial result", slog.String("error", err.Error()))
	}

	// Execute web action
	notificationMessage, err := h.executeWebAction(ctx, payload)
	executionTime := time.Since(startTime)

	if err != nil {
		// Mark result as failed
		result.MarkFailure(err.Error(), executionTime.Milliseconds())
		if saveErr := h.resultRepo.SaveResult(ctx, result); saveErr != nil {
			h.logger.Warn("failed to save failed result", slog.String("error", saveErr.Error()))
		}

		// Mark message as failed
		h.markMessageFailed(ctx, message, err.Error())

		return fmt.Errorf("web action execution failed: %w", err)
	}

	// Mark result as successful
	result.MarkSuccess(200, notificationMessage, executionTime.Milliseconds())
	if err := h.resultRepo.SaveResult(ctx, result); err != nil {
		h.logger.Warn("failed to save successful result", slog.String("error", err.Error()))
	}

	// Mark message as completed
	message.MarkCompleted()
	if err := h.messageRepo.SaveMessage(ctx, message); err != nil {
		h.logger.Warn("failed to update message status to completed", slog.String("error", err.Error()))
	}

	// Publish notification message
	if err := h.publishNotification(ctx, message, notificationMessage); err != nil {
		h.logger.Error("failed to publish notification",
			slog.String("error", err.Error()),
		)
		// Don't return error - the web action succeeded even if notification failed
	}

	h.logger.Info("web action completed successfully",
		slog.String("message_id", message.ID),
		slog.String("action", payload.Action.String()),
		slog.Duration("execution_time", executionTime),
	)

	return nil
}

// executeWebAction executes a web action using the appropriate handler
func (h *Handler) executeWebAction(ctx context.Context, payload *models.WebActionPayload) (string, error) {
	// Get handler for action type
	handler, err := h.handlerRegistry.GetHandler(payload.Action)
	if err != nil {
		return "", fmt.Errorf("no handler available: %w", err)
	}

	// Execute action
	return handler.Execute(ctx, payload)
}

// publishNotification publishes a notification message to SNS
func (h *Handler) publishNotification(ctx context.Context, originalMessage *models.Message, notificationContent string) error {
	// Create notification message
	notificationMsg := models.NewMessage(
		"web-action-processor",
		originalMessage.Stage,
		models.MessageTypeScheduled,
		notificationContent,
	)

	// Save notification message
	if err := h.messageRepo.SaveMessage(ctx, notificationMsg); err != nil {
		return fmt.Errorf("failed to save notification message: %w", err)
	}

	// Publish to SNS
	if err := h.snsPublisher.PublishMessage(ctx, notificationMsg); err != nil {
		return fmt.Errorf("failed to publish notification to SNS: %w", err)
	}

	h.logger.Info("notification published",
		slog.String("notification_message_id", notificationMsg.ID),
		slog.String("original_message_id", originalMessage.ID),
	)

	return nil
}

// markMessageFailed marks a message as failed
func (h *Handler) markMessageFailed(ctx context.Context, message *models.Message, errorMessage string) {
	message.MarkFailed(errorMessage)
	if err := h.messageRepo.SaveMessage(ctx, message); err != nil {
		h.logger.Error("failed to mark message as failed",
			slog.String("message_id", message.ID),
			slog.String("error", err.Error()),
		)
	}
}
